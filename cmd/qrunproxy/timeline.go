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

type trackCell struct {
	cell  TimelineCell
	isGap bool
}

type timelineBuilder struct {
	blocks    map[string]Block
	trackIDs  []string
	tracks    []Track
	trackIdx  map[string]int
	startSigs map[string][]TriggerTarget

	trackCells  [][]trackCell
	constraints []constraint
	exclusives  []exclusiveGroup
}

func BuildTimeline(show Show) (Timeline, error) {
	b := &timelineBuilder{
		blocks:    map[string]Block{},
		trackIdx:  map[string]int{},
		startSigs: map[string][]TriggerTarget{},
	}

	b.trackIDs = append(b.trackIDs, cueTrackID)
	b.tracks = append(b.tracks, Track{ID: cueTrackID, Name: "Cue"})
	b.trackIdx[cueTrackID] = 0
	for i, track := range show.Tracks {
		b.trackIDs = append(b.trackIDs, track.ID)
		b.tracks = append(b.tracks, track)
		b.trackIdx[track.ID] = i + 1
	}
	for _, block := range show.Blocks {
		b.blocks[block.ID] = block
	}
	for _, trigger := range show.Triggers {
		if trigger.Source.Signal == "START" {
			b.startSigs[trigger.Source.Block] = append(b.startSigs[trigger.Source.Block], trigger.Targets...)
		}
	}

	b.trackCells = make([][]trackCell, len(b.trackIDs))

	if err := b.buildCells(show); err != nil {
		return Timeline{}, err
	}

	b.assignRows()

	rows := b.renderRows()

	blocks := map[string]Block{}
	for id, block := range b.blocks {
		blocks[id] = block
	}

	return Timeline{
		Tracks: b.tracks,
		Blocks: blocks,
		Rows:   rows,
	}, nil
}

func (b *timelineBuilder) appendCell(trackID string, cell TimelineCell) cellID {
	idx, ok := b.trackIdx[trackID]
	if !ok {
		return cellID{-1, -1}
	}
	i := len(b.trackCells[idx])
	b.trackCells[idx] = append(b.trackCells[idx], trackCell{cell: cell})
	return cellID{track: idx, index: i}
}

func (b *timelineBuilder) addConstraint(kind string, a, b2 cellID) {
	if a.track < 0 || b2.track < 0 {
		return
	}
	b.constraints = append(b.constraints, constraint{kind: kind, a: a, b: b2})
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

type blockCells struct {
	start   cellID
	title   cellID
	fadeOut cellID
	end     cellID
}

func (b *timelineBuilder) buildCells(show Show) error {
	cells := map[string]blockCells{}

	for _, block := range show.Blocks {
		trackID := b.getTrack(block.ID)
		if trackID == "" {
			continue
		}
		if block.Type == "cue" {
			cueID := b.appendCell(trackID, TimelineCell{
				BlockID: block.ID,
				IsStart: true,
				IsEnd:   true,
				Event:   "GO",
			})
			cells[block.ID] = blockCells{start: cueID, title: cueID, fadeOut: cueID, end: cueID}
			continue
		}
		startID := b.appendCell(trackID, TimelineCell{BlockID: block.ID, IsStart: true, Event: "START"})
		titleID := b.appendCell(trackID, TimelineCell{BlockID: block.ID, IsTitle: true})
		fadeOutID := b.appendCell(trackID, TimelineCell{BlockID: block.ID, Event: "FADE_OUT"})
		endID := b.appendCell(trackID, TimelineCell{BlockID: block.ID, IsEnd: true, Event: "END"})
		cells[block.ID] = blockCells{start: startID, title: titleID, fadeOut: fadeOutID, end: endID}
	}

	for _, trigger := range show.Triggers {
		if trigger.Source.Signal == "START" {
			continue
		}

		sourceCell := cells[trigger.Source.Block]
		var sourceID cellID
		switch trigger.Source.Signal {
		case "GO":
			sourceID = sourceCell.start
		case "END":
			sourceID = sourceCell.end
		case "FADE_OUT":
			sourceID = sourceCell.fadeOut
		default:
			continue
		}

		group := exclusiveGroup{members: []cellID{sourceID}}
		hasCrossTrack := false

		allTargets := b.expandTargets(trigger.Targets)
		for _, target := range allTargets {
			tc := cells[target.Block]
			var targetID cellID
			switch target.Hook {
			case "START":
				targetID = tc.start
			case "END":
				targetID = tc.end
			case "FADE_OUT":
				targetID = tc.fadeOut
			default:
				continue
			}
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

	return nil
}

func (b *timelineBuilder) expandTargets(targets []TriggerTarget) []TriggerTarget {
	var result []TriggerTarget
	seen := map[string]bool{}
	queue := append([]TriggerTarget(nil), targets...)

	for len(queue) > 0 {
		target := queue[0]
		queue = queue[1:]
		if seen[target.Block] {
			continue
		}
		seen[target.Block] = true
		result = append(result, target)
		if target.Hook == "START" {
			if chained, has := b.startSigs[target.Block]; has {
				queue = append(queue, chained...)
			}
		}
	}
	return result
}

func (b *timelineBuilder) setSignal(id cellID) {
	if id.track < 0 {
		return
	}
	b.trackCells[id.track][id.index].cell.IsSignal = true
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
			tc := b.trackCells[trackIdx][row]
			if tc.isGap || tc.cell.BlockID == "" {
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
	newCells := make([]trackCell, 0, len(cells)+1)
	newCells = append(newCells, cells[:beforeIndex]...)
	newCells = append(newCells, trackCell{isGap: true})
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
		rows[r].Cells = make([]TimelineCell, len(b.trackIDs))
	}

	for trackIdx, cells := range b.trackCells {
		activeBlock := ""
		for r := 0; r < maxLen; r++ {
			if r < len(cells) {
				tc := cells[r]
				if tc.isGap {
					if activeBlock != "" {
						rows[r].Cells[trackIdx] = TimelineCell{BlockID: activeBlock}
					}
				} else {
					rows[r].Cells[trackIdx] = tc.cell
					if tc.cell.BlockID != "" && !tc.cell.IsEnd {
						activeBlock = tc.cell.BlockID
					} else if tc.cell.IsEnd {
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
