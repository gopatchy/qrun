package main

import "fmt"

const cueTrackID = "_cue"

type TimelineTrack struct {
	*Track
	Cells []*TimelineCell `json:"cells"`
}

type Timeline struct {
	Tracks []*TimelineTrack  `json:"tracks"`
	Blocks map[string]*Block `json:"blocks"`

	show        *Show                     `json:"-"`
	trackIdx    map[string]*TimelineTrack `json:"-"`
	cellIdx     map[cellKey]*TimelineCell `json:"-"`
	constraints []constraint              `json:"-"`
	exclusives  []exclusiveGroup          `json:"-"`
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
	Type     CellType       `json:"type"`
	BlockID  string         `json:"block_id,omitempty"`
	Event    string         `json:"event,omitempty"`
	row      int            `json:"-"`
	track    *TimelineTrack `json:"-"`
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

type constraintKind string

const (
	constraintSameRow constraintKind = "same_row"
)

type constraint struct {
	kind constraintKind
	a    *TimelineCell
	b    *TimelineCell
}

func (c constraint) satisfied() bool {
	switch c.kind {
	case constraintSameRow:
		return c.a.row == c.b.row
	default:
		panic("invalid constraint kind: " + string(c.kind))
	}
}

func (c constraint) String() string {
	switch c.kind {
	case constraintSameRow:
		return fmt.Sprintf("same_row(%s, %s)", c.a, c.b)
	default:
		panic("invalid constraint kind: " + string(c.kind))
	}
}

type exclusiveGroup struct {
	members []*TimelineCell
}

func (g exclusiveGroup) satisfied(tracks []*TimelineTrack) bool {
	row := g.members[0].row
	memberTracks := map[*TimelineTrack]bool{}
	for _, m := range g.members {
		memberTracks[m.track] = true
		if m.row != row {
			return true
		}
	}
	for _, t := range tracks {
		if memberTracks[t] {
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

type cellKey struct {
	blockID string
	event   string
}

func BuildTimeline(show *Show) (Timeline, error) {
	tl := Timeline{
		show:     show,
		Blocks:   map[string]*Block{},
		trackIdx: map[string]*TimelineTrack{},
		cellIdx:  map[cellKey]*TimelineCell{},
	}

	cueTrack := &TimelineTrack{Track: &Track{ID: cueTrackID, Name: "Cue"}}
	tl.Tracks = append(tl.Tracks, cueTrack)
	tl.trackIdx[cueTrackID] = cueTrack
	for _, track := range show.Tracks {
		tt := &TimelineTrack{Track: track}
		tl.Tracks = append(tl.Tracks, tt)
		tl.trackIdx[track.ID] = tt
	}
	for _, block := range show.Blocks {
		if block.Type == "cue" {
			block.Track = cueTrackID
		}
		tl.Blocks[block.ID] = block
	}

	endChains := map[string]bool{}
	for _, trigger := range show.Triggers {
		if trigger.Source.Signal != "END" {
			continue
		}
		srcTrack := tl.Blocks[trigger.Source.Block].Track
		for _, target := range trigger.Targets {
			if target.Hook == "START" && tl.Blocks[target.Block].Track == srcTrack {
				endChains[trigger.Source.Block] = true
			}
		}
	}

	tl.buildCells(endChains)
	tl.buildConstraints()
	if err := tl.assignRows(); err != nil {
		return Timeline{}, err
	}

	return tl, nil
}

func (tl *Timeline) addConstraint(kind constraintKind, a, b *TimelineCell) {
	tl.constraints = append(tl.constraints, constraint{kind: kind, a: a, b: b})
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

func (tl *Timeline) buildCells(endChains map[string]bool) {
	lastOnTrack := map[string]*Block{}
	for _, block := range tl.show.Blocks {
		lastOnTrack[block.Track] = block
	}

	for _, block := range tl.show.Blocks {
		track := tl.trackIdx[block.Track]
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
		if block.Type != "cue" && lastOnTrack[block.Track] != block {
			if endChains[block.ID] {
				track.appendCells(&TimelineCell{Type: CellChain})
			} else {
				track.appendCells(&TimelineCell{Type: CellGap})
			}
		}
	}
}

func (tl *Timeline) buildConstraints() {
	for _, trigger := range tl.show.Triggers {
		source := tl.findCell(trigger.Source.Block, trigger.Source.Signal)

		group := exclusiveGroup{members: []*TimelineCell{source}}

		for _, target := range trigger.Targets {
			t := tl.findCell(target.Block, target.Hook)
			if source.track != t.track {
				tl.addConstraint(constraintSameRow, source, t)
				source.Type = CellSignal
			}
			group.members = append(group.members, t)
		}
		tl.exclusives = append(tl.exclusives, group)
	}
}

func (tl *Timeline) assignRows() error {
	for range 1000000 {
		if tl.enforceConstraints() {
			continue
		}
		if tl.enforceExclusives() {
			continue
		}
		return nil
	}
	for _, c := range tl.constraints {
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

func (tl *Timeline) enforceConstraints() bool {
	for _, c := range tl.constraints {
		if c.satisfied() {
			continue
		}
		switch c.kind {
		case constraintSameRow:
			if c.a.row < c.b.row {
				tl.insertGap(c.a.track, c.a.row)
			} else {
				tl.insertGap(c.b.track, c.b.row)
			}
		default:
			panic("invalid constraint kind: " + string(c.kind))
		}
		return true
	}
	return false
}

func (tl *Timeline) enforceExclusives() bool {
	for _, g := range tl.exclusives {
		if g.satisfied(tl.Tracks) {
			continue
		}
		row := g.members[0].row
		memberTracks := map[*TimelineTrack]bool{}
		for _, m := range g.members {
			memberTracks[m.track] = true
		}
		for _, t := range tl.Tracks {
			if memberTracks[t] {
				continue
			}
			if !t.cellTypeAt(row, CellEvent, CellTitle, CellSignal) {
				continue
			}
			tl.insertGap(t, row)
			return true
		}
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
		for _, t := range tl.Tracks {
			if t == track {
				continue
			}
			if beforeIndex >= len(t.Cells) {
				continue
			}
			tl.removeGapAt(t, beforeIndex)
		}
		return
	}

	gap := &TimelineCell{Type: CellGap, row: beforeIndex, track: track}
	if beforeIndex > 0 {
		prev := track.Cells[beforeIndex-1]
		if prev.BlockID != "" && prev.Event != "END" && prev.Event != "GO" {
			gap.Type = CellContinuation
			gap.BlockID = prev.BlockID
		}
	}

	track.Cells = append(track.Cells[:beforeIndex], append([]*TimelineCell{gap}, track.Cells[beforeIndex:]...)...)
	tl.reindexRowsFrom(track, beforeIndex+1)
}
