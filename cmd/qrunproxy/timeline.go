package main

import (
	"fmt"
	"sort"
)

const (
	cueTrackID = "_cue"
	intMax     = int(^uint(0) >> 1)
)

type Show struct {
	Tracks   []Track   `json:"tracks"`
	Blocks   []Block   `json:"blocks"`
	Triggers []Trigger `json:"triggers"`
}

type Track struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Block struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Track string `json:"track,omitempty"`
	Name  string `json:"name"`
	Loop  bool   `json:"loop,omitempty"`
}

type Trigger struct {
	Source  TriggerSource   `json:"source"`
	Targets []TriggerTarget `json:"targets"`
}

type TriggerSource struct {
	Block  string `json:"block"`
	Signal string `json:"signal"`
}

type TriggerTarget struct {
	Block string `json:"block"`
	Hook  string `json:"hook"`
}

type Timeline struct {
	Tracks []Track          `json:"tracks"`
	Blocks map[string]Block `json:"blocks"`
	Rows   []TimelineRow    `json:"rows"`
}

type TimelineRow struct {
	Cells []TimelineCell `json:"cells"`
}

type TimelineCell struct {
	BlockID  string `json:"block_id,omitempty"`
	IsStart  bool   `json:"is_start,omitempty"`
	IsEnd    bool   `json:"is_end,omitempty"`
	Event    string `json:"event,omitempty"`
	IsTitle  bool   `json:"is_title,omitempty"`
	IsSignal bool   `json:"is_signal,omitempty"`
}

type timelineBuilder struct {
	show         Show
	blocks       map[string]Block
	trackIDs     []string
	tracks       []Track
	startSigs    map[string][]TriggerTarget
	hasEndSignal map[string]bool
	active       map[string]string
	pending      map[string]struct{}
	rows         []TimelineRow
	noTitle      map[int]struct{}
}

type blockRange struct {
	first int
	last  int
	start int
	end   int
}

type titleInfo struct {
	blockID string
	track   string
	s       int
	e       int
	pos     int
}

type titleGroup struct {
	pos    int
	s      int
	e      int
	titles []titleInfo
}

type orderedHooks struct {
	order  []string
	values map[string]string
}

func newOrderedHooks() orderedHooks {
	return orderedHooks{values: map[string]string{}}
}

func (o *orderedHooks) Set(block, hook string) {
	if _, ok := o.values[block]; !ok {
		o.order = append(o.order, block)
	}
	o.values[block] = hook
}

func (o orderedHooks) Get(block string) (string, bool) {
	v, ok := o.values[block]
	return v, ok
}

func (o orderedHooks) Len() int {
	return len(o.order)
}

func (o orderedHooks) ForEach(fn func(block, hook string)) {
	for _, block := range o.order {
		fn(block, o.values[block])
	}
}

func BuildTimeline(show Show) (Timeline, error) {
	builder := &timelineBuilder{
		show:         show,
		blocks:       map[string]Block{},
		startSigs:    map[string][]TriggerTarget{},
		hasEndSignal: map[string]bool{},
		active:       map[string]string{},
		pending:      map[string]struct{}{},
		rows:         make([]TimelineRow, 0, len(show.Triggers)+8),
		noTitle:      map[int]struct{}{},
	}

	builder.trackIDs = append(builder.trackIDs, cueTrackID)
	builder.tracks = append(builder.tracks, Track{ID: cueTrackID, Name: "Cue"})
	for _, track := range show.Tracks {
		builder.trackIDs = append(builder.trackIDs, track.ID)
		builder.tracks = append(builder.tracks, track)
	}
	for _, block := range show.Blocks {
		builder.blocks[block.ID] = block
	}
	for _, trigger := range show.Triggers {
		if trigger.Source.Signal == "START" {
			builder.startSigs[trigger.Source.Block] = append(builder.startSigs[trigger.Source.Block], trigger.Targets...)
		}
		if trigger.Source.Signal == "END" {
			builder.hasEndSignal[trigger.Source.Block] = true
		}
	}

	if err := builder.buildRows(); err != nil {
		return Timeline{}, err
	}

	rows := builder.insertTitleRows()
	blocks := map[string]Block{}
	for id, block := range builder.blocks {
		blocks[id] = block
	}

	return Timeline{
		Tracks: builder.tracks,
		Blocks: blocks,
		Rows:   rows,
	}, nil
}

func (b *timelineBuilder) buildRows() error {
	for i := 0; i < len(b.show.Triggers); i++ {
		trigger := b.show.Triggers[i]
		if trigger.Source.Signal == "START" {
			continue
		}

		if b.isChain(trigger) {
			if _, hasStartSignals := b.startSigs[trigger.Targets[0].Block]; hasStartSignals {
				if err := b.processChainWithStartSignals(trigger); err != nil {
					return err
				}
				continue
			}

			nextIndex, err := b.processChainBatch(i)
			if err != nil {
				return err
			}
			i = nextIndex
			continue
		}

		if err := b.processSignal(trigger); err != nil {
			return err
		}
	}

	b.flushPending()

	activeEvents := map[string]TimelineCell{}
	for trackID, blockID := range b.active {
		if trackID == cueTrackID {
			continue
		}
		b.setCell(activeEvents, trackID, TimelineCell{BlockID: blockID})
	}
	if len(activeEvents) > 0 {
		b.addRow(b.mkCells(activeEvents))
	}

	return nil
}

func (b *timelineBuilder) processChainWithStartSignals(trigger Trigger) error {
	b.flushPending()

	sourceID := trigger.Source.Block
	targetID := trigger.Targets[0].Block
	trackID := b.getTrack(targetID)
	if trackID == "" {
		return fmt.Errorf("missing track for block %s", targetID)
	}

	ends := map[string]TimelineCell{}
	if (b.active[trackID] == sourceID) || b.hasPending(sourceID) {
		delete(b.pending, sourceID)
		delete(b.active, trackID)
		b.setCell(ends, trackID, TimelineCell{BlockID: sourceID, IsEnd: true, Event: "END"})
	}
	if len(ends) > 0 {
		b.addRow(b.mkCells(ends))
	}

	b.active[trackID] = targetID
	starts := map[string]TimelineCell{}
	sideEffects := newOrderedHooks()
	expanded := b.expandTargets(b.startSigs[targetID])
	expanded.ForEach(func(block, hook string) {
		sideEffects.Set(block, hook)
	})
	b.setCell(starts, trackID, TimelineCell{BlockID: targetID, IsStart: true, Event: "START", IsSignal: true})

	b.noTitle[len(b.rows)-1] = struct{}{}
	sideEffects.ForEach(func(block, hook string) {
		b.applySideEffect(starts, block, hook)
	})

	b.addRow(b.mkCells(starts))
	return nil
}

func (b *timelineBuilder) processChainBatch(startIndex int) (int, error) {
	trigger := b.show.Triggers[startIndex]
	batch := []Trigger{trigger}
	tracks := map[string]struct{}{b.getTrack(trigger.Source.Block): {}}
	j := startIndex + 1

	for j < len(b.show.Triggers) {
		candidate := b.show.Triggers[j]
		if candidate.Source.Signal == "START" {
			j++
			continue
		}
		if !b.isChain(candidate) {
			break
		}
		candidateTrack := b.getTrack(candidate.Source.Block)
		if _, exists := tracks[candidateTrack]; exists {
			break
		}
		if _, hasStartSignals := b.startSigs[candidate.Targets[0].Block]; hasStartSignals {
			break
		}
		tracks[candidateTrack] = struct{}{}
		batch = append(batch, candidate)
		j++
	}

	b.flushPending()

	ends := map[string]TimelineCell{}
	for _, chain := range batch {
		sourceID := chain.Source.Block
		trackID := b.getTrack(sourceID)
		if trackID == "" {
			return startIndex, fmt.Errorf("missing track for block %s", sourceID)
		}
		if (b.active[trackID] == sourceID) || b.hasPending(sourceID) {
			delete(b.pending, sourceID)
			delete(b.active, trackID)
			b.setCell(ends, trackID, TimelineCell{BlockID: sourceID, IsEnd: true, Event: "END"})
		}
	}
	if len(ends) > 0 {
		b.addRow(b.mkCells(ends))
	}

	starts := map[string]TimelineCell{}
	sideEffects := newOrderedHooks()
	for _, chain := range batch {
		targetID := chain.Targets[0].Block
		trackID := b.getTrack(targetID)
		if trackID == "" {
			return startIndex, fmt.Errorf("missing track for block %s", targetID)
		}
		b.active[trackID] = targetID
		_, hasStartSignals := b.startSigs[targetID]
		if hasStartSignals {
			expanded := b.expandTargets(b.startSigs[targetID])
			expanded.ForEach(func(block, hook string) {
				sideEffects.Set(block, hook)
			})
		}
		b.setCell(starts, trackID, TimelineCell{BlockID: targetID, IsStart: true, Event: "START", IsSignal: hasStartSignals})
	}

	b.noTitle[len(b.rows)-1] = struct{}{}
	sideEffects.ForEach(func(block, hook string) {
		b.applySideEffect(starts, block, hook)
	})

	b.addRow(b.mkCells(starts))

	return j - 1, nil
}

func (b *timelineBuilder) processSignal(trigger Trigger) error {
	b.flushPending()

	isCue := trigger.Source.Signal == "GO"
	targets := newOrderedHooks()
	for _, target := range trigger.Targets {
		targets.Set(target.Block, target.Hook)
	}
	expanded := b.expandTargets(trigger.Targets)
	expanded.ForEach(func(block, hook string) {
		targets.Set(block, hook)
	})

	events := map[string]TimelineCell{}
	directEnds := map[string]struct{}{}

	if isCue {
		b.setCell(events, cueTrackID, TimelineCell{
			BlockID:  trigger.Source.Block,
			IsStart:  true,
			IsEnd:    true,
			Event:    trigger.Source.Signal,
			IsSignal: true,
		})
	} else {
		sourceTrack := b.getTrack(trigger.Source.Block)
		if sourceTrack != "" && sourceTrack != cueTrackID {
			b.setCell(events, sourceTrack, TimelineCell{
				BlockID:  trigger.Source.Block,
				IsEnd:    trigger.Source.Signal == "END",
				Event:    trigger.Source.Signal,
				IsSignal: true,
			})
		}
	}

	targets.ForEach(func(blockID, hook string) {
		trackID := b.getTrack(blockID)
		if trackID == "" {
			return
		}
		switch hook {
		case "START":
			b.active[trackID] = blockID
			b.setCell(events, trackID, TimelineCell{
				BlockID: blockID,
				IsStart: true,
				Event:   "START",
			})
		case "END":
			b.setCell(events, trackID, TimelineCell{
				BlockID: blockID,
				IsEnd:   true,
				Event:   "END",
			})
			cell := events[trackID]
			if cell.BlockID == blockID && cell.Event == "END" {
				directEnds[blockID] = struct{}{}
			}
		case "FADE_OUT":
			b.pending[blockID] = struct{}{}
			b.setCell(events, trackID, TimelineCell{
				BlockID: blockID,
				Event:   "FADE_OUT",
			})
		}
	})

	b.addRow(b.mkCells(events))

	for blockID := range directEnds {
		delete(b.active, b.getTrack(blockID))
	}

	if !isCue {
		if trigger.Source.Signal == "FADE_OUT" {
			b.pending[trigger.Source.Block] = struct{}{}
		}
		if trigger.Source.Signal == "END" {
			delete(b.active, b.getTrack(trigger.Source.Block))
			delete(b.pending, trigger.Source.Block)
		}
	}

	return nil
}

func (b *timelineBuilder) isChain(trigger Trigger) bool {
	if trigger.Source.Signal != "END" || len(trigger.Targets) != 1 {
		return false
	}
	return trigger.Targets[0].Hook == "START" && b.getTrack(trigger.Source.Block) == b.getTrack(trigger.Targets[0].Block)
}

func (b *timelineBuilder) expandTargets(targets []TriggerTarget) orderedHooks {
	result := newOrderedHooks()
	queue := append([]TriggerTarget(nil), targets...)

	for len(queue) > 0 {
		target := queue[0]
		queue = queue[1:]

		if _, exists := result.Get(target.Block); exists {
			continue
		}
		result.Set(target.Block, target.Hook)

		if target.Hook == "START" {
			if chained, has := b.startSigs[target.Block]; has {
				queue = append(queue, chained...)
			}
		}
	}

	return result
}

func (b *timelineBuilder) applySideEffect(events map[string]TimelineCell, blockID, hook string) {
	trackID := b.getTrack(blockID)
	if trackID == "" {
		return
	}

	switch hook {
	case "START":
		b.active[trackID] = blockID
		b.setCell(events, trackID, TimelineCell{BlockID: blockID, IsStart: true, Event: "START"})
	case "END":
		b.setCell(events, trackID, TimelineCell{BlockID: blockID, IsEnd: true, Event: "END"})
		delete(b.active, trackID)
	case "FADE_OUT":
		b.pending[blockID] = struct{}{}
		b.setCell(events, trackID, TimelineCell{BlockID: blockID, Event: "FADE_OUT"})
	}
}

func (b *timelineBuilder) flushPending() {
	toEnd := make([]string, 0, len(b.pending))
	for blockID := range b.pending {
		if !b.hasEndSignal[blockID] {
			toEnd = append(toEnd, blockID)
		}
	}
	if len(toEnd) == 0 {
		return
	}
	sort.Strings(toEnd)

	events := map[string]TimelineCell{}
	for _, blockID := range toEnd {
		trackID := b.getTrack(blockID)
		if trackID == "" {
			continue
		}
		b.setCell(events, trackID, TimelineCell{BlockID: blockID, IsEnd: true, Event: "END"})
	}
	if len(events) > 0 {
		b.addRow(b.mkCells(events))
	}

	for _, blockID := range toEnd {
		delete(b.active, b.getTrack(blockID))
		delete(b.pending, blockID)
	}
}

func (b *timelineBuilder) hasPending(blockID string) bool {
	_, ok := b.pending[blockID]
	return ok
}

func (b *timelineBuilder) getTrack(blockID string) string {
	block, ok := b.blocks[blockID]
	if !ok {
		return ""
	}
	if block.Type == "cue" {
		return cueTrackID
	}
	return block.Track
}

func (b *timelineBuilder) setCell(events map[string]TimelineCell, trackID string, cell TimelineCell) {
	existing, ok := events[trackID]
	if !ok {
		events[trackID] = cell
		return
	}
	events[trackID] = mergeCell(existing, cell)
}

func mergeCell(existing, next TimelineCell) TimelineCell {
	if existing.IsTitle {
		return existing
	}
	if existing.BlockID == "" {
		return next
	}
	if next.BlockID == "" {
		return existing
	}
	if existing.BlockID != next.BlockID {
		return existing
	}

	existing.IsStart = existing.IsStart || next.IsStart
	existing.IsEnd = existing.IsEnd || next.IsEnd
	if existing.Event == "" {
		existing.Event = next.Event
	}

	if next.Event == "" || existing.Event == next.Event {
		existing.IsSignal = existing.IsSignal || next.IsSignal
	}

	if next.IsTitle {
		existing.IsTitle = true
	}

	return existing
}

func (b *timelineBuilder) mkCells(events map[string]TimelineCell) []TimelineCell {
	cells := make([]TimelineCell, 0, len(b.trackIDs))
	for _, trackID := range b.trackIDs {
		if cell, ok := events[trackID]; ok {
			cells = append(cells, cell)
		} else {
			cells = append(cells, b.midCell(trackID))
		}
	}
	return cells
}

func (b *timelineBuilder) midCell(trackID string) TimelineCell {
	if blockID, ok := b.active[trackID]; ok {
		return TimelineCell{BlockID: blockID}
	}
	return TimelineCell{}
}

func (b *timelineBuilder) addRow(cells []TimelineCell) {
	if len(b.rows) > 0 {
		last := b.rows[len(b.rows)-1]
		if b.sameRowType(last.Cells, cells) {
			merge := true
			for i := 0; i < len(cells); i++ {
				if hasEventOrCue(cells[i]) && hasEventOrCue(last.Cells[i]) {
					merge = false
					break
				}
			}
			if merge {
				for i := 0; i < len(cells); i++ {
					if hasEventOrCue(cells[i]) {
						last.Cells[i] = cells[i]
					}
				}
				b.rows[len(b.rows)-1] = last
				return
			}
		}
	}

	b.rows = append(b.rows, TimelineRow{Cells: cells})
}

func hasEventOrCue(cell TimelineCell) bool {
	return cell.Event != ""
}

func (b *timelineBuilder) rowType(cells []TimelineCell) (cue bool, signal bool) {
	for _, cell := range cells {
		if cell.BlockID == "" || cell.Event == "" {
			continue
		}
		block, ok := b.blocks[cell.BlockID]
		if ok && block.Type == "cue" {
			// Cue rows take precedence for type classification.
			return true, false
		}
		if cell.IsSignal {
			signal = true
		}
	}
	return false, signal
}

func (b *timelineBuilder) sameRowType(a, c []TimelineCell) bool {
	cueA, signalA := b.rowType(a)
	cueC, signalC := b.rowType(c)
	return cueA == cueC && signalA == signalC
}

func (b *timelineBuilder) insertTitleRows() []TimelineRow {
	ranges := map[string]*blockRange{}
	order := make([]string, 0, len(b.blocks))

	for rowIndex, row := range b.rows {
		for _, cell := range row.Cells {
			if cell.BlockID == "" {
				continue
			}
			rng, ok := ranges[cell.BlockID]
			if !ok {
				rng = &blockRange{first: rowIndex, last: rowIndex, start: -1, end: intMax}
				ranges[cell.BlockID] = rng
				order = append(order, cell.BlockID)
			}
			rng.last = rowIndex
			if cell.Event == "START" {
				rng.start = maxInt(rng.start, rowIndex)
			}
			if cell.Event == "END" || cell.Event == "FADE_OUT" {
				rng.end = minInt(rng.end, rowIndex)
			}
		}
	}

	titles := make([]titleInfo, 0, len(ranges))
	for _, blockID := range order {
		block := b.blocks[blockID]
		if block.Type == "cue" {
			continue
		}
		rng := ranges[blockID]
		s := rng.first
		if rng.start >= 0 {
			s = rng.start
		}
		e := rng.last
		if rng.end != intMax {
			e = rng.end
		}
		titleEnd := e
		if rng.end != intMax {
			titleEnd = rng.end - 1
		}
		titles = append(titles, titleInfo{
			blockID: blockID,
			track:   b.getTrack(blockID),
			s:       s,
			e:       titleEnd,
			pos:     (s + titleEnd) / 2,
		})
	}
	sort.SliceStable(titles, func(i, j int) bool {
		return titles[i].pos < titles[j].pos
	})

	groups := make([]titleGroup, 0, len(titles))
	for _, title := range titles {
		bestIndex := -1
		bestDistance := intMax
		for i := 0; i < len(groups); i++ {
			group := groups[i]
			intersectStart := maxInt(group.s, title.s)
			intersectEnd := minInt(group.e, title.e)
			if intersectStart > intersectEnd {
				continue
			}
			if hasTrack(group, title.track) {
				continue
			}

			candidate := group.pos
			if candidate < intersectStart || candidate > intersectEnd {
				candidate = (intersectStart + intersectEnd) / 2
			}
			if b.isNoTitle(candidate) {
				found := false
				for r := candidate + 1; r <= intersectEnd; r++ {
					if !b.isNoTitle(r) {
						candidate = r
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			distance := absInt(candidate - title.pos)
			if distance < bestDistance {
				bestDistance = distance
				bestIndex = i
			}
		}

		if bestIndex >= 0 {
			group := groups[bestIndex]
			group.s = maxInt(group.s, title.s)
			group.e = minInt(group.e, title.e)
			if group.pos < group.s || group.pos > group.e {
				group.pos = (group.s + group.e) / 2
			}
			if b.isNoTitle(group.pos) {
				for r := group.pos + 1; r <= group.e; r++ {
					if !b.isNoTitle(r) {
						group.pos = r
						break
					}
				}
			}
			group.titles = append(group.titles, title)
			groups[bestIndex] = group
		} else {
			pos := title.pos
			if b.isNoTitle(pos) {
				for r := pos + 1; r <= title.e; r++ {
					if !b.isNoTitle(r) {
						pos = r
						break
					}
				}
			}
			groups = append(groups, titleGroup{pos: pos, s: title.s, e: title.e, titles: []titleInfo{title}})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].pos < groups[j].pos
	})

	finalRows := make([]TimelineRow, 0, len(b.rows)+len(groups))
	groupIndex := 0
	for rowIndex, row := range b.rows {
		finalRows = append(finalRows, row)
		for groupIndex < len(groups) && groups[groupIndex].pos == rowIndex {
			group := groups[groupIndex]
			cells := make([]TimelineCell, 0, len(b.trackIDs))
			for col, trackID := range b.trackIDs {
				if title, ok := findTitleForTrack(group, trackID); ok {
					cells = append(cells, TimelineCell{BlockID: title.blockID, IsTitle: true})
					continue
				}
				prev := row.Cells[col]
				if prev.BlockID != "" && !prev.IsEnd {
					cells = append(cells, TimelineCell{BlockID: prev.BlockID})
				} else {
					cells = append(cells, TimelineCell{})
				}
			}
			finalRows = append(finalRows, TimelineRow{Cells: cells})
			groupIndex++
		}
	}

	return finalRows
}

func (b *timelineBuilder) isNoTitle(row int) bool {
	_, ok := b.noTitle[row]
	return ok
}

func hasTrack(group titleGroup, track string) bool {
	for _, title := range group.titles {
		if title.track == track {
			return true
		}
	}
	return false
}

func findTitleForTrack(group titleGroup, track string) (titleInfo, bool) {
	for _, title := range group.titles {
		if title.track == track {
			return title, true
		}
	}
	return titleInfo{}, false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
