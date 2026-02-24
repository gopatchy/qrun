package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

const cueTrackID = "_cue"

type TimelineTrack struct {
	*Track
	Cells []*TimelineCell `json:"cells"`
}

type Timeline struct {
	Tracks []*TimelineTrack  `json:"tracks"`
	Blocks map[string]*Block `json:"blocks"`

	show       *Show                     `json:"-"`
	trackIdx   map[string]*TimelineTrack `json:"-"`
	cellIdx    map[cellKey]*TimelineCell `json:"-"`
	sameRows   []sameRowConstraint       `json:"-"`
	exclusives []exclusiveGroup          `json:"-"`
	debugW     io.Writer                 `json:"-"`
}

type CellType string

const (
	CellEvent        CellType = "event"
	CellTitle        CellType = "title"
	CellContinuation CellType = "continuation"
	CellGap          CellType = "gap"
	CellChain        CellType = "chain"
	CellSignal       CellType = "signal"
)

type TimelineCell struct {
	Type    CellType       `json:"type"`
	BlockID string         `json:"block_id,omitempty"`
	Event   string         `json:"event,omitempty"`
	row     int            `json:"-"`
	track   *TimelineTrack `json:"-"`
}

func (t *TimelineTrack) cellTypeAt(index int, types ...CellType) bool {
	if index < 0 || index >= len(t.Cells) {
		return false
	}
	for _, typ := range types {
		if t.Cells[index].Type == typ {
			return true
		}
	}
	return false
}

func (c *TimelineCell) String() string {
	return fmt.Sprintf("%s/%s@%s:r%d", c.BlockID, c.Event, c.track.ID, c.row)
}

type sameRowConstraint struct {
	a *TimelineCell
	b *TimelineCell
}

func (c sameRowConstraint) satisfied() bool {
	return c.a.row == c.b.row
}

func (c sameRowConstraint) String() string {
	return fmt.Sprintf("same_row(%s, %s)", c.a, c.b)
}

type exclusiveGroup struct {
	members      []*TimelineCell
	memberTracks map[*TimelineTrack]bool
}

func (g exclusiveGroup) satisfied(tracks []*TimelineTrack) bool {
	row := g.members[0].row
	for _, m := range g.members {
		if m.row != row {
			return true
		}
	}
	for _, t := range tracks {
		if g.memberTracks[t] {
			continue
		}
		if t.cellTypeAt(row, CellEvent, CellTitle, CellSignal) {
			return false
		}
	}
	return true
}

func (g exclusiveGroup) String() string {
	s := "exclusive("
	for i, m := range g.members {
		if i > 0 {
			s += ", "
		}
		s += m.String()
	}
	return s + ")"
}

func (tl *Timeline) debugf(format string, args ...any) {
	if tl.debugW != nil {
		fmt.Fprintf(tl.debugW, format+"\n", args...)
	}
}

func (tl *Timeline) debugState() {
	if tl.debugW == nil {
		return
	}
	fmt.Fprintf(tl.debugW, "=== state ===\n")
	for _, t := range tl.Tracks {
		var parts []string
		for _, c := range t.Cells {
			switch c.Type {
			case CellEvent, CellSignal:
				parts = append(parts, fmt.Sprintf("r%d:%s/%s", c.row, c.BlockID, c.Event))
			case CellTitle:
				parts = append(parts, fmt.Sprintf("r%d:title(%s)", c.row, c.BlockID))
			default:
				parts = append(parts, fmt.Sprintf("r%d:%s", c.row, c.Type))
			}
		}
		fmt.Fprintf(tl.debugW, "  %s: [%s]\n", t.ID, strings.Join(parts, " "))
	}
	for _, c := range tl.sameRows {
		sat := "OK"
		if !c.satisfied() {
			sat = "UNSATISFIED"
		}
		fmt.Fprintf(tl.debugW, "  constraint %s %s\n", c, sat)
	}
	for _, g := range tl.exclusives {
		sat := "OK"
		if !g.satisfied(tl.Tracks) {
			sat = "UNSATISFIED"
		}
		fmt.Fprintf(tl.debugW, "  exclusive %s %s\n", g, sat)
	}
	fmt.Fprintf(tl.debugW, "=============\n")
}

type cellKey struct {
	blockID string
	event   string
}

func BuildTimeline(show *Show) (Timeline, error) {
	return BuildTimelineDebug(show, nil)
}

func BuildTimelineDebug(show *Show, debugW io.Writer) (Timeline, error) {
	tl := Timeline{
		show:     show,
		Blocks:   map[string]*Block{},
		trackIdx: map[string]*TimelineTrack{},
		cellIdx:  map[cellKey]*TimelineCell{},
		debugW:   debugW,
	}

	tl.buildTracks()
	tl.indexBlocks()
	tl.linkTriggers()
	tl.computeWeights()
	tl.buildCells()
	tl.buildConstraints()
	if err := tl.assignRows(); err != nil {
		return Timeline{}, err
	}

	return tl, nil
}

func (tl *Timeline) buildTracks() {
	cueTrack := &TimelineTrack{Track: &Track{ID: cueTrackID, Name: "Cue"}}
	tl.Tracks = append(tl.Tracks, cueTrack)
	tl.trackIdx[cueTrackID] = cueTrack

	for _, track := range tl.show.Tracks {
		tt := &TimelineTrack{Track: track}
		tl.Tracks = append(tl.Tracks, tt)
		tl.trackIdx[track.ID] = tt
	}
}

func (tl *Timeline) indexBlocks() {
	for _, block := range tl.show.Blocks {
		if block.Type == "cue" {
			block.Track = cueTrackID
		}
		tl.Blocks[block.ID] = block
	}
}

func (tl *Timeline) linkTriggers() {
	for _, block := range tl.show.Blocks {
		block.triggers = nil
	}
	for _, trigger := range tl.show.Triggers {
		trigger.Source.block = tl.Blocks[trigger.Source.Block]
		trigger.Source.block.triggers = append(trigger.Source.block.triggers, trigger)
		for i := range trigger.Targets {
			trigger.Targets[i].block = tl.Blocks[trigger.Targets[i].Block]
		}
	}
}

func (tl *Timeline) computeWeights() {
	cueEndedBlocks := map[string]bool{}
	for _, trigger := range tl.show.Triggers {
		if trigger.Source.block.Type != "cue" {
			continue
		}
		for _, target := range trigger.Targets {
			if target.Hook == "END" || target.Hook == "FADE_OUT" {
				cueEndedBlocks[target.Block] = true
			}
		}
	}

	cueIdx := uint64(0)
	for _, block := range tl.show.Blocks {
		if block.Type == "cue" {
			tl.setWeightDFS(block, cueIdx<<32, cueEndedBlocks)
			cueIdx++
		}
	}
}

func (tl *Timeline) setWeightDFS(b *Block, base uint64, cueEndedBlocks map[string]bool) {
	if cueEndedBlocks[b.ID] {
		b.weight = base
	} else {
		b.weight = base + 1
	}
	for _, trigger := range b.triggers {
		for _, target := range trigger.Targets {
			tl.setWeightDFS(target.block, base, cueEndedBlocks)
		}
	}
}

func (tl *Timeline) findEndChains() map[string]bool {
	endChains := map[string]bool{}
	for _, trigger := range tl.show.Triggers {
		if trigger.Source.Signal != "END" {
			continue
		}
		for _, target := range trigger.Targets {
			if target.Hook == "START" && target.block.Track == trigger.Source.block.Track {
				endChains[trigger.Source.Block] = true
			}
		}
	}
	return endChains
}

func (tl *Timeline) addSameRow(a, b *TimelineCell) {
	tl.sameRows = append(tl.sameRows, sameRowConstraint{a: a, b: b})
}

func (track *TimelineTrack) appendCells(cells ...*TimelineCell) {
	for _, c := range cells {
		c.row = len(track.Cells)
		c.track = track
		track.Cells = append(track.Cells, c)
	}
}

func getCueCells(block *Block) []*TimelineCell {
	return []*TimelineCell{{
		Type:    CellEvent,
		BlockID: block.ID,
		Event:   "GO",
	}}
}

func getBlockCells(block *Block) []*TimelineCell {
	return []*TimelineCell{
		{Type: CellEvent, BlockID: block.ID, Event: "START"},
		{Type: CellTitle, BlockID: block.ID},
		{Type: CellEvent, BlockID: block.ID, Event: "FADE_OUT"},
		{Type: CellEvent, BlockID: block.ID, Event: "END"},
	}
}

func (tl *Timeline) findCell(blockID, event string) *TimelineCell {
	if c := tl.cellIdx[cellKey{blockID: blockID, event: event}]; c != nil {
		return c
	}
	panic("cell not found: " + blockID + " " + event)
}

func blockHeight(b *Block) int {
	if b.Type == "cue" {
		return 0
	}
	return 4
}

func eventOffset(event string) int {
	switch event {
	case "GO":
		return 0
	case "START":
		return 0
	case "FADE_OUT":
		return 2
	case "END":
		return 3
	default:
		return 0
	}
}

func (tl *Timeline) buildCells() {
	cueIdx := 0
	for _, block := range tl.show.Blocks {
		if block.Type == "cue" {
			block.topRow = cueIdx
			cueIdx++
		} else {
			block.topRow = 0
		}
	}

	for range 1000 {
		changed := false

		for _, trigger := range tl.show.Triggers {
			srcRow := trigger.Source.block.topRow + eventOffset(trigger.Source.Signal)
			for _, target := range trigger.Targets {
				targetRow := srcRow - eventOffset(target.Hook)
				if target.block.topRow < targetRow {
					target.block.topRow = targetRow
					changed = true
				}
			}
		}

		for _, track := range tl.Tracks {
			trackBlocks := tl.blocksByTrack(track.ID)
			slices.SortFunc(trackBlocks, func(a, b *Block) int {
				if a.topRow != b.topRow {
					return a.topRow - b.topRow
				}
				if a.weight != b.weight {
					if a.weight < b.weight {
						return -1
					}
					return 1
				}
				return 0
			})
			for i := 1; i < len(trackBlocks); i++ {
				prev := trackBlocks[i-1]
				cur := trackBlocks[i]
				minRow := prev.topRow + blockHeight(prev) + 1
				if cur.topRow < minRow {
					cur.topRow = minRow
					changed = true
				}
			}
		}

		if !changed {
			break
		}
	}

	for _, block := range tl.show.Blocks {
		if block.Type != "cue" {
			continue
		}
		maxRow := block.topRow
		for _, trigger := range block.triggers {
			for _, target := range trigger.Targets {
				r := target.block.topRow + eventOffset(target.Hook)
				if r > maxRow {
					maxRow = r
				}
			}
		}
		block.topRow = maxRow
	}

	tl.emitCells()
	tl.debugState()
}

func (tl *Timeline) blocksByTrack(trackID string) []*Block {
	var blocks []*Block
	for _, block := range tl.show.Blocks {
		if block.Track == trackID {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func (tl *Timeline) emitCells() {
	endChains := tl.findEndChains()

	type trackEntry struct {
		block *Block
	}
	trackBlocks := map[string][]*Block{}
	for _, block := range tl.show.Blocks {
		trackBlocks[block.Track] = append(trackBlocks[block.Track], block)
	}

	for trackID, blocks := range trackBlocks {
		track := tl.trackIdx[trackID]
		slices.SortFunc(blocks, func(a, b *Block) int {
			return a.topRow - b.topRow
		})

		row := 0
		for i, block := range blocks {
			for row < block.topRow {
				if block.Type != "cue" {
					track.appendCells(&TimelineCell{Type: CellGap})
				}
				row++
			}

			var cells []*TimelineCell
			switch block.Type {
			case "cue":
				cells = getCueCells(block)
			default:
				cells = getBlockCells(block)
			}
			track.appendCells(cells...)
			for _, c := range cells {
				if c.Event == "" {
					continue
				}
				tl.cellIdx[cellKey{blockID: c.BlockID, event: c.Event}] = c
			}
			row += len(cells)

			isLast := i == len(blocks)-1
			if !isLast && block.Type != "cue" {
				if endChains[block.ID] {
					track.appendCells(&TimelineCell{Type: CellChain})
				} else {
					track.appendCells(&TimelineCell{Type: CellGap})
				}
				row++
			}
		}
	}
}

func (tl *Timeline) buildConstraints() {
	for _, trigger := range tl.show.Triggers {
		source := tl.findCell(trigger.Source.Block, trigger.Source.Signal)

		group := exclusiveGroup{
			members:      []*TimelineCell{source},
			memberTracks: map[*TimelineTrack]bool{source.track: true},
		}

		for _, target := range trigger.Targets {
			t := tl.findCell(target.Block, target.Hook)
			if source.track != t.track {
				tl.addSameRow(source, t)
				source.Type = CellSignal
			}
			group.members = append(group.members, t)
			group.memberTracks[t.track] = true
		}
		tl.exclusives = append(tl.exclusives, group)
	}
}

func (tl *Timeline) assignRows() error {
	tl.debugState()
	for i := range 1000000 {
		if tl.enforceConstraints(i) {
			continue
		}
		if tl.enforceExclusives(i) {
			continue
		}
		tl.debugf("converged after %d iterations", i)
		return nil
	}
	for _, c := range tl.sameRows {
		if !c.satisfied() {
			return fmt.Errorf("assignRows: unsatisfied %s", c)
		}
	}
	for _, g := range tl.exclusives {
		if !g.satisfied(tl.Tracks) {
			return fmt.Errorf("assignRows: unsatisfied %s", g)
		}
	}
	return fmt.Errorf("assignRows: did not converge")
}

func (tl *Timeline) enforceConstraints(iter int) bool {
	for _, c := range tl.sameRows {
		if c.satisfied() {
			continue
		}
		if c.a.row < c.b.row {
			tl.debugf("iter %d: constraint %s: insert gap on %s before r%d", iter, c, c.a.track.ID, c.a.row)
			tl.insertGap(c.a.track, c.a.row)
		} else {
			tl.debugf("iter %d: constraint %s: insert gap on %s before r%d", iter, c, c.b.track.ID, c.b.row)
			tl.insertGap(c.b.track, c.b.row)
		}
		return true
	}
	return false
}

func (tl *Timeline) enforceExclusives(iter int) bool {
	for _, g := range tl.exclusives {
		if g.satisfied(tl.Tracks) {
			continue
		}
		row := g.members[0].row
		tl.debugf("iter %d: exclusive %s: split at r%d", iter, g, row)
		for _, t := range tl.Tracks {
			if row >= len(t.Cells) {
				continue
			}
			if g.memberTracks[t] {
				tl.debugf("  member %s: insertGapInt before r%d", t.ID, row)
				tl.insertGapInt(t, row)
			} else {
				tl.debugf("  non-member %s: insertGapInt before r%d", t.ID, row+1)
				tl.insertGapInt(t, row+1)
			}
		}
		return true
	}
	return false
}

func (tl *Timeline) isAllRemovableGapRow(row int, except *TimelineTrack) bool {
	for _, t := range tl.Tracks {
		if t == except {
			continue
		}
		if row >= len(t.Cells) {
			continue
		}
		if !t.cellTypeAt(row, CellGap, CellChain, CellContinuation) {
			return false
		}
		hasBefore := t.cellTypeAt(row-1, CellEvent, CellTitle, CellSignal)
		hasAfter := t.cellTypeAt(row+1, CellEvent, CellTitle, CellSignal)
		if hasBefore && hasAfter {
			return false
		}
	}
	return true
}

func (tl *Timeline) removeGapAt(track *TimelineTrack, index int) {
	track.Cells = append(track.Cells[:index], track.Cells[index+1:]...)
	tl.reindexRowsFrom(track, index)
}

func (tl *Timeline) reindexRowsFrom(track *TimelineTrack, start int) {
	for i := start; i < len(track.Cells); i++ {
		track.Cells[i].row = i
	}
}

func (tl *Timeline) insertGap(track *TimelineTrack, beforeIndex int) {
	if tl.isAllRemovableGapRow(beforeIndex, track) {
		tl.debugf("  insertGap(%s, r%d): removable row, removing from other tracks", track.ID, beforeIndex)
		for _, t := range tl.Tracks {
			if t == track {
				continue
			}
			if beforeIndex >= len(t.Cells) {
				continue
			}
			tl.debugf("    removeGap %s r%d (%s)", t.ID, beforeIndex, t.Cells[beforeIndex].Type)
			tl.removeGapAt(t, beforeIndex)
		}
		return
	}
	tl.debugf("  insertGap(%s, r%d): inserting", track.ID, beforeIndex)
	tl.insertGapInt(track, beforeIndex)
}

func (tl *Timeline) insertGapInt(track *TimelineTrack, beforeIndex int) {
	gap := &TimelineCell{Type: CellGap, row: beforeIndex, track: track}
	if beforeIndex > 0 {
		prev := track.Cells[beforeIndex-1]
		if prev.Type == CellChain {
			gap.Type = CellChain
		} else if prev.BlockID != "" && prev.Event != "END" && prev.Event != "GO" {
			gap.Type = CellContinuation
			gap.BlockID = prev.BlockID
		}
	}

	track.Cells = slices.Insert(track.Cells, beforeIndex, gap)
	tl.reindexRowsFrom(track, beforeIndex+1)
}
