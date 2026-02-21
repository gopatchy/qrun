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
	IsBreak  bool           `json:"-"`
	row      int            `json:"-"`
	track    *TimelineTrack `json:"-"`
}

type constraint struct {
	kind string
	a    *TimelineCell
	b    *TimelineCell
}

type exclusiveGroup struct {
	members []*TimelineCell
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
		if block.Type != "cue" && !endChains[block.ID] && lastOnTrack[block.Track] != block {
			track.appendCells(&TimelineCell{IsGap: true, IsBreak: true})
		}
	}
}

func (tl *Timeline) buildConstraints() {
	for _, trigger := range tl.show.Triggers {
		source := tl.findCell(trigger.Source.Block, trigger.Source.Signal)

		group := exclusiveGroup{members: []*TimelineCell{source}}

		for _, target := range trigger.Targets {
			t := tl.findCell(target.Block, target.Hook)
			if source.track == t.track {
				tl.addConstraint("next_row", source, t)
			} else {
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
		switch c.kind {
		case "same_row":
			if c.a.row != c.b.row {
				return fmt.Errorf("assignRows: unsatisfied %s constraint: %s/%s (row %d) vs %s/%s (row %d)", c.kind, c.a.BlockID, c.a.Event, c.a.row, c.b.BlockID, c.b.Event, c.b.row)
			}
		case "next_row":
			if c.b.row <= c.a.row {
				return fmt.Errorf("assignRows: unsatisfied %s constraint: %s/%s (row %d) must follow %s/%s (row %d)", c.kind, c.b.BlockID, c.b.Event, c.b.row, c.a.BlockID, c.a.Event, c.a.row)
			}
		}
	}
	return fmt.Errorf("assignRows: did not converge")
}

func (tl *Timeline) enforceConstraints() bool {
	for _, c := range tl.constraints {
		switch c.kind {
		case "same_row":
			if c.a.row < c.b.row {
				tl.insertGap(c.a.track, c.a.row)
				return true
			} else if c.b.row < c.a.row {
				tl.insertGap(c.b.track, c.b.row)
				return true
			}
		case "next_row":
			if c.b.row <= c.a.row {
				tl.insertGap(c.b.track, c.b.row)
				return true
			}
		}
	}
	return false
}

func (tl *Timeline) enforceExclusives() bool {
	for _, g := range tl.exclusives {
		row := g.members[0].row
		allAligned := true
		memberTracks := map[*TimelineTrack]bool{}
		for _, m := range g.members {
			memberTracks[m.track] = true
			if m.row != row {
				allAligned = false
			}
		}
		if !allAligned {
			continue
		}
		for _, t := range tl.Tracks {
			if memberTracks[t] {
				continue
			}
			if row >= len(t.Cells) {
				continue
			}
			c := t.Cells[row]
			if c.IsGap || c.BlockID == "" {
				continue
			}
			tl.insertGap(t, row)
			return true
		}
	}
	return false
}

func (tl *Timeline) shiftBreakDownOne(track *TimelineTrack, row int) {
	below := track.Cells[row+1]
	track.Cells[row].IsBreak = false
	below.IsBreak = true
}

func (tl *Timeline) canShiftBreakDownOne(track *TimelineTrack, row int) bool {
	if row+1 >= len(track.Cells) {
		return false
	}
	below := track.Cells[row+1]
	if !below.IsGap || below.IsBreak {
		return false
	}
	return true
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
		if !c.IsGap {
			return false
		}
		if c.IsBreak && !tl.canShiftBreakDownOne(t, row) {
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

func (tl *Timeline) gapInsertionPoint(track *TimelineTrack, index int) int {
	for {
		blocked := false
		for _, c := range tl.constraints {
			if c.kind == "next_row" && c.a.track == track && c.b.track == track && c.a.row == index-1 && c.b.row == index {
				index = c.a.row
				blocked = true
				break
			}
		}
		if !blocked {
			return index
		}
	}
}

func (tl *Timeline) insertGap(track *TimelineTrack, beforeIndex int) {
	beforeIndex = tl.gapInsertionPoint(track, beforeIndex)

	if tl.isAllRemovableGapRow(beforeIndex, track) {
		for _, t := range tl.Tracks {
			if t == track {
				continue
			}
			if beforeIndex >= len(t.Cells) {
				continue
			}
			c := t.Cells[beforeIndex]
			if c.IsBreak {
				tl.shiftBreakDownOne(t, beforeIndex)
				c = t.Cells[beforeIndex]
			}
			if c.IsGap && !c.IsBreak {
				tl.removeGapAt(t, beforeIndex)
			}
		}
		return
	}

	gap := &TimelineCell{IsGap: true, row: beforeIndex, track: track}
	for i := beforeIndex - 1; i >= 0; i-- {
		c := track.Cells[i]
		if c.IsGap {
			continue
		}
		if c.BlockID != "" && !c.IsEnd {
			gap.BlockID = c.BlockID
		}
		break
	}

	track.Cells = append(track.Cells[:beforeIndex], append([]*TimelineCell{gap}, track.Cells[beforeIndex:]...)...)
	tl.reindexRowsFrom(track, beforeIndex+1)
}
