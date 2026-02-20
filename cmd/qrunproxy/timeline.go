package main

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
	IsGap    bool   `json:"-"`
}

type cellID struct {
	track int
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

type timelineBuilder struct {
	show      Show
	blocks    map[string]Block
	tracks    []Track
	trackIdx  map[string]int
	trackCells  [][]TimelineCell
	constraints []constraint
	exclusives  []exclusiveGroup
}

func newTimelineBuilder(show Show) *timelineBuilder {
	b := &timelineBuilder{
		show:      show,
		blocks:    map[string]Block{},
		trackIdx:  map[string]int{},
	}

	b.tracks = append(b.tracks, Track{ID: cueTrackID, Name: "Cue"})
	b.trackIdx[cueTrackID] = 0
	for i, track := range show.Tracks {
		b.tracks = append(b.tracks, track)
		b.trackIdx[track.ID] = i + 1
	}
	for _, block := range show.Blocks {
		if block.Type == "cue" {
			block.Track = cueTrackID
		}
		b.blocks[block.ID] = block
	}

	b.trackCells = make([][]TimelineCell, len(b.tracks))

	return b
}

func BuildTimeline(show Show) (Timeline, error) {
	b := newTimelineBuilder(show)

	b.buildCells()
	b.buildConstraints()
	b.assignRows()

	return Timeline{
		Tracks: b.tracks,
		Blocks: b.blocks,
		Rows:   b.renderRows(),
	}, nil
}

func (b *timelineBuilder) addConstraint(kind string, a, b2 cellID) {
	b.constraints = append(b.constraints, constraint{kind: kind, a: a, b: b2})
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

func (b *timelineBuilder) findCell(blockID, event string) cellID {
	track := b.trackIdx[b.blocks[blockID].Track]
	for i, c := range b.trackCells[track] {
		if !c.IsGap && c.BlockID == blockID && c.Event == event {
			return cellID{track: track, index: i}
		}
	}
	panic("cell not found: " + blockID + " " + event)
}

func (b *timelineBuilder) endChainsSameTrack(blockID string) bool {
	trackID := b.blocks[blockID].Track
	for _, trigger := range b.show.Triggers {
		if trigger.Source.Block != blockID || trigger.Source.Signal != "END" {
			continue
		}
		for _, target := range trigger.Targets {
			if target.Hook == "START" && b.blocks[target.Block].Track == trackID {
				return true
			}
		}
	}
	return false
}

func (b *timelineBuilder) buildCells() {
	for _, sb := range b.show.Blocks {
		block := b.blocks[sb.ID]
		idx := b.trackIdx[block.Track]
		var cells []TimelineCell
		switch block.Type {
		case "cue":
			cells = getCueCells(block)
		default:
			cells = getBlockCells(block)
		}
		b.trackCells[idx] = append(b.trackCells[idx], cells...)
		if block.Type != "cue" && !b.endChainsSameTrack(block.ID) {
			b.trackCells[idx] = append(b.trackCells[idx], TimelineCell{IsGap: true})
		}
	}
}

func (b *timelineBuilder) buildConstraints() {
	for _, trigger := range b.show.Triggers {
		sourceID := b.findCell(trigger.Source.Block, trigger.Source.Signal)

		group := exclusiveGroup{members: []cellID{sourceID}}
		hasCrossTrack := false

		for _, target := range trigger.Targets {
			targetID := b.findCell(target.Block, target.Hook)
			if sourceID.track == targetID.track {
				b.addConstraint("next_row", sourceID, targetID)
			} else {
				b.addConstraint("same_row", sourceID, targetID)
				hasCrossTrack = true
			}
			group.members = append(group.members, targetID)
		}

		if hasCrossTrack {
			b.setSignal(sourceID)
		}
		b.exclusives = append(b.exclusives, group)
	}
}

func (b *timelineBuilder) setSignal(id cellID) {
	b.trackCells[id.track][id.index].IsSignal = true
}

func (b *timelineBuilder) assignRows() {
	for iter := 0; iter < 10000; iter++ {
		found := false
		for _, c := range b.constraints {
			aRow := b.rowOf(c.a)
			bRow := b.rowOf(c.b)

			switch c.kind {
			case "same_row":
				if aRow < bRow {
					b.insertGap(c.a.track, c.a.index)
					found = true
				} else if bRow < aRow {
					b.insertGap(c.b.track, c.b.index)
					found = true
				}
			case "next_row":
				if bRow <= aRow {
					b.insertGap(c.b.track, c.b.index)
					found = true
				}
			}
			if found {
				break
			}
		}
		if !found {
			found = b.enforceExclusives()
		}
		if !found {
			break
		}
	}
}

func (b *timelineBuilder) enforceExclusives() bool {
	for _, g := range b.exclusives {
		if len(g.members) == 0 {
			continue
		}
		row := b.rowOf(g.members[0])
		allAligned := true
		memberTracks := map[int]bool{}
		for _, m := range g.members {
			memberTracks[m.track] = true
			if b.rowOf(m) != row {
				allAligned = false
			}
		}
		if !allAligned {
			continue
		}
		for trackIdx := range b.trackCells {
			if memberTracks[trackIdx] {
				continue
			}
			if row >= len(b.trackCells[trackIdx]) {
				continue
			}
			c := b.trackCells[trackIdx][row]
			if c.IsGap || c.BlockID == "" {
				continue
			}
			b.insertGap(trackIdx, row)
			return true
		}
	}
	return false
}

func (b *timelineBuilder) rowOf(id cellID) int {
	return id.index
}

func (b *timelineBuilder) insertGap(track, beforeIndex int) {
	cells := b.trackCells[track]
	newCells := make([]TimelineCell, 0, len(cells)+1)
	newCells = append(newCells, cells[:beforeIndex]...)
	newCells = append(newCells, TimelineCell{IsGap: true})
	newCells = append(newCells, cells[beforeIndex:]...)
	b.trackCells[track] = newCells

	for i := range b.constraints {
		c := &b.constraints[i]
		if c.a.track == track && c.a.index >= beforeIndex {
			c.a.index++
		}
		if c.b.track == track && c.b.index >= beforeIndex {
			c.b.index++
		}
	}
	for i := range b.exclusives {
		for j := range b.exclusives[i].members {
			m := &b.exclusives[i].members[j]
			if m.track == track && m.index >= beforeIndex {
				m.index++
			}
		}
	}
}

func (b *timelineBuilder) renderRows() []TimelineRow {
	maxLen := 0
	for _, cells := range b.trackCells {
		if len(cells) > maxLen {
			maxLen = len(cells)
		}
	}

	rows := make([]TimelineRow, maxLen)
	for r := range rows {
		rows[r].Cells = make([]TimelineCell, len(b.tracks))
	}

	for trackIdx, cells := range b.trackCells {
		activeBlock := ""
		for r := 0; r < maxLen; r++ {
			if r < len(cells) {
				c := cells[r]
				if c.IsGap {
					if activeBlock != "" {
						rows[r].Cells[trackIdx] = TimelineCell{BlockID: activeBlock}
					}
				} else {
					rows[r].Cells[trackIdx] = c
					if c.BlockID != "" && !c.IsEnd {
						activeBlock = c.BlockID
					} else if c.IsEnd {
						activeBlock = ""
					}
				}
			} else if activeBlock != "" {
				rows[r].Cells[trackIdx] = TimelineCell{BlockID: activeBlock}
			}
		}
	}

	return rows
}
