package main

import "fmt"

const (
	cueTrackID = "_cue"
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

type TimelineTrack struct {
	Track
	Cells []TimelineCell `json:"cells"`
}

type Timeline struct {
	Tracks []*TimelineTrack  `json:"tracks"`
	Blocks map[string]Block `json:"blocks"`

	show        Show                       `json:"-"`
	trackIdx    map[string]*TimelineTrack  `json:"-"`
	constraints []constraint               `json:"-"`
	exclusives  []exclusiveGroup           `json:"-"`
}

type TimelineCell struct {
	BlockID  string `json:"block_id,omitempty"`
	IsStart  bool   `json:"is_start,omitempty"`
	IsEnd    bool   `json:"is_end,omitempty"`
	Event    string `json:"event,omitempty"`
	IsTitle  bool   `json:"is_title,omitempty"`
	IsSignal bool   `json:"is_signal,omitempty"`
	IsGap    bool   `json:"-"`
	IsBreak  bool   `json:"-"`
}

type cellID struct {
	track *TimelineTrack
	index int
}

type constraint struct {
	kind string
	a    cellID
	b    cellID
}

type exclusiveGroup struct {
	members []cellID
}

func validateShow(show Show) error {
	startTargeted := map[string]bool{}
	for _, trigger := range show.Triggers {
		for _, target := range trigger.Targets {
			if target.Hook == "START" {
				startTargeted[target.Block] = true
			}
		}
	}

	for _, block := range show.Blocks {
		if block.Type == "cue" {
			continue
		}
		if !startTargeted[block.ID] {
			return fmt.Errorf("block %q has no trigger for its START", block.ID)
		}
	}

	return nil
}

func BuildTimeline(show Show) (Timeline, error) {
	if err := validateShow(show); err != nil {
		return Timeline{}, err
	}

	tl := Timeline{
		show:     show,
		Blocks:   map[string]Block{},
		trackIdx: map[string]*TimelineTrack{},
	}

	cueTrack := &TimelineTrack{Track: Track{ID: cueTrackID, Name: "Cue"}}
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

	tl.buildCells()
	tl.buildConstraints()
	tl.assignRows()

	return tl, nil
}

func (tl *Timeline) addConstraint(kind string, a, b cellID) {
	tl.constraints = append(tl.constraints, constraint{kind: kind, a: a, b: b})
}

func getCueCells(block Block) []TimelineCell {
	return []TimelineCell{{
		BlockID: block.ID,
		IsStart: true,
		IsEnd:   true,
		Event:   "GO",
	}}
}

func getBlockCells(block Block) []TimelineCell {
	return []TimelineCell{
		{BlockID: block.ID, IsStart: true, Event: "START"},
		{BlockID: block.ID, IsTitle: true},
		{BlockID: block.ID, Event: "FADE_OUT"},
		{BlockID: block.ID, IsEnd: true, Event: "END"},
	}
}

func (tl *Timeline) findCell(blockID, event string) cellID {
	track := tl.trackIdx[tl.Blocks[blockID].Track]
	for i, c := range track.Cells {
		if !c.IsGap && c.BlockID == blockID && c.Event == event {
			return cellID{track: track, index: i}
		}
	}
	panic("cell not found: " + blockID + " " + event)
}

func (tl *Timeline) endChainsSameTrack(blockID string) bool {
	trackID := tl.Blocks[blockID].Track
	for _, trigger := range tl.show.Triggers {
		if trigger.Source.Block != blockID || trigger.Source.Signal != "END" {
			continue
		}
		for _, target := range trigger.Targets {
			if target.Hook == "START" && tl.Blocks[target.Block].Track == trackID {
				return true
			}
		}
	}
	return false
}

func (tl *Timeline) buildCells() {
	for _, sb := range tl.show.Blocks {
		block := tl.Blocks[sb.ID]
		track := tl.trackIdx[block.Track]
		var cells []TimelineCell
		switch block.Type {
		case "cue":
			cells = getCueCells(block)
		default:
			cells = getBlockCells(block)
		}
		track.Cells = append(track.Cells, cells...)
		if block.Type != "cue" && !tl.endChainsSameTrack(block.ID) {
			track.Cells = append(track.Cells, TimelineCell{IsGap: true, IsBreak: true})
		}
	}
}

func (tl *Timeline) buildConstraints() {
	for _, trigger := range tl.show.Triggers {
		sourceID := tl.findCell(trigger.Source.Block, trigger.Source.Signal)

		group := exclusiveGroup{members: []cellID{sourceID}}
		hasCrossTrack := false

		for _, target := range trigger.Targets {
			targetID := tl.findCell(target.Block, target.Hook)
			if sourceID.track == targetID.track {
				tl.addConstraint("next_row", sourceID, targetID)
			} else {
				tl.addConstraint("same_row", sourceID, targetID)
				hasCrossTrack = true
			}
			group.members = append(group.members, targetID)
		}

		if hasCrossTrack {
			sourceID.track.Cells[sourceID.index].IsSignal = true
		}
		tl.exclusives = append(tl.exclusives, group)
	}
}

func (tl *Timeline) assignRows() {
	for {
		found := false
		for _, c := range tl.constraints {
			aRow := tl.rowOf(c.a)
			bRow := tl.rowOf(c.b)

			switch c.kind {
			case "same_row":
				if aRow < bRow {
					tl.insertGap(c.a.track, c.a.index)
					found = true
				} else if bRow < aRow {
					tl.insertGap(c.b.track, c.b.index)
					found = true
				}
			case "next_row":
				if bRow <= aRow {
					tl.insertGap(c.b.track, c.b.index)
					found = true
				}
			}
			if found {
				break
			}
		}
		if !found {
			found = tl.enforceExclusives()
		}
		if !found {
			break
		}
	}
}

func (tl *Timeline) enforceExclusives() bool {
	for _, g := range tl.exclusives {
		if len(g.members) == 0 {
			continue
		}
		row := tl.rowOf(g.members[0])
		allAligned := true
		memberTracks := map[*TimelineTrack]bool{}
		for _, m := range g.members {
			memberTracks[m.track] = true
			if tl.rowOf(m) != row {
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

func (tl *Timeline) rowOf(id cellID) int {
	return id.index
}

func (tl *Timeline) isAllGapRow(row int, except *TimelineTrack) bool {
	for _, t := range tl.Tracks {
		if t == except {
			continue
		}
		if row >= len(t.Cells) {
			continue
		}
		if !t.Cells[row].IsGap {
			return false
		}
	}
	return true
}

func (tl *Timeline) removeGapAt(track *TimelineTrack, index int) {
	track.Cells = append(track.Cells[:index], track.Cells[index+1:]...)

	for i := range tl.constraints {
		c := &tl.constraints[i]
		if c.a.track == track && c.a.index > index {
			c.a.index--
		}
		if c.b.track == track && c.b.index > index {
			c.b.index--
		}
	}
	for i := range tl.exclusives {
		for j := range tl.exclusives[i].members {
			m := &tl.exclusives[i].members[j]
			if m.track == track && m.index > index {
				m.index--
			}
		}
	}
}

func (tl *Timeline) insertGap(track *TimelineTrack, beforeIndex int) {
	for {
		blocked := false
		for _, c := range tl.constraints {
			if c.kind == "next_row" && c.a.track == track && c.b.track == track && c.a.index == beforeIndex-1 && c.b.index == beforeIndex {
				beforeIndex = c.a.index
				blocked = true
				break
			}
		}
		if !blocked {
			break
		}
	}

	if tl.isAllGapRow(beforeIndex, track) {
		for _, t := range tl.Tracks {
			if t == track {
				continue
			}
			if beforeIndex >= len(t.Cells) {
				continue
			}
			if t.Cells[beforeIndex].IsGap {
				tl.removeGapAt(t, beforeIndex)
			}
		}
		return
	}

	gap := TimelineCell{IsGap: true}
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

	cells := track.Cells
	newCells := make([]TimelineCell, 0, len(cells)+1)
	newCells = append(newCells, cells[:beforeIndex]...)
	newCells = append(newCells, gap)
	newCells = append(newCells, cells[beforeIndex:]...)
	track.Cells = newCells

	for i := range tl.constraints {
		c := &tl.constraints[i]
		if c.a.track == track && c.a.index >= beforeIndex {
			c.a.index++
		}
		if c.b.track == track && c.b.index >= beforeIndex {
			c.b.index++
		}
	}
	for i := range tl.exclusives {
		for j := range tl.exclusives[i].members {
			m := &tl.exclusives[i].members[j]
			if m.track == track && m.index >= beforeIndex {
				m.index++
			}
		}
	}
}
