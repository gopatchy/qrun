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

type TimelineCell struct {
	BlockID  string         `json:"block_id,omitempty"`
	IsStart  bool           `json:"is_start,omitempty"`
	IsEnd    bool           `json:"is_end,omitempty"`
	Event    string         `json:"event,omitempty"`
	IsTitle  bool           `json:"is_title,omitempty"`
	IsSignal bool           `json:"is_signal,omitempty"`
	IsGap    bool           `json:"-"`
	IsChain  bool           `json:"-"`
	row      int            `json:"-"`
	track    *TimelineTrack `json:"-"`
}

func (c *TimelineCell) String() string {
	return fmt.Sprintf("%s/%s@%s:r%d", c.BlockID, c.Event, c.track.ID, c.row)
}

type constraint struct {
	kind string
	a    *TimelineCell
	b    *TimelineCell
}

func (c constraint) satisfied() bool {
	switch c.kind {
	case "same_row":
		return c.a.row == c.b.row
	case "next_row":
		return c.b.row > c.a.row
	}
	return true
}

func (c constraint) String() string {
	switch c.kind {
	case "same_row":
		return fmt.Sprintf("same_row(%s, %s)", c.a, c.b)
	case "next_row":
		return fmt.Sprintf("next_row(%s -> %s)", c.a, c.b)
	}
	return fmt.Sprintf("%s(%s, %s)", c.kind, c.a, c.b)
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
		if row >= len(t.Cells) {
			continue
		}
		c := t.Cells[row]
		if c.IsGap || c.IsChain || c.BlockID == "" {
			continue
		}
		return false
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

func (tl *Timeline) addConstraint(kind string, a, b *TimelineCell) {
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
		BlockID: block.ID,
		IsStart: true,
		IsEnd:   true,
		Event:   "GO",
	}}
}

func getBlockCells(block *Block) []*TimelineCell {
	return []*TimelineCell{
		{BlockID: block.ID, IsStart: true, Event: "START"},
		{BlockID: block.ID, IsTitle: true},
		{BlockID: block.ID, Event: "FADE_OUT"},
		{BlockID: block.ID, IsEnd: true, Event: "END"},
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
				track.appendCells(&TimelineCell{IsChain: true})
			} else {
				track.appendCells(&TimelineCell{IsGap: true})
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
				tl.addConstraint("same_row", source, t)
				source.IsSignal = true
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
		case "same_row":
			if c.a.row < c.b.row {
				tl.insertGap(c.a.track, c.a.row)
			} else {
				tl.insertGap(c.b.track, c.b.row)
			}
		case "next_row":
			tl.insertGap(c.b.track, c.b.row)
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
			if row >= len(t.Cells) {
				continue
			}
			c := t.Cells[row]
			if c.IsGap || c.IsChain || c.BlockID == "" {
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
		c := t.Cells[row]
		if !c.IsGap && !c.IsChain {
			return false
		}
		hasBefore := row > 0 && t.Cells[row-1].BlockID != "" && !t.Cells[row-1].IsGap && !t.Cells[row-1].IsChain
		hasAfter := row+1 < len(t.Cells) && t.Cells[row+1].BlockID != "" && !t.Cells[row+1].IsGap && !t.Cells[row+1].IsChain
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

	gap := &TimelineCell{IsGap: true, row: beforeIndex, track: track}
	if beforeIndex > 0 {
		prev := track.Cells[beforeIndex-1]
		if prev.BlockID != "" && !prev.IsEnd {
			gap.BlockID = prev.BlockID
		}
	}

	track.Cells = append(track.Cells[:beforeIndex], append([]*TimelineCell{gap}, track.Cells[beforeIndex:]...)...)
	tl.reindexRowsFrom(track, beforeIndex+1)
}
