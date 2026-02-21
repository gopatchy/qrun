package main

import "fmt"

func GenerateMockShow(numTracks, numCues, numBlocks int) *Show {
	show := &Show{}

	for i := range numTracks {
		show.Tracks = append(show.Tracks, &Track{
			ID:   fmt.Sprintf("track_%d", i),
			Name: fmt.Sprintf("Track %d", i),
		})
	}

	for i := range numCues {
		show.Blocks = append(show.Blocks, &Block{
			ID:   fmt.Sprintf("cue_%d", i),
			Type: "cue",
			Name: fmt.Sprintf("Cue %d", i),
		})
	}

	blocksByTrack := make([][]*Block, numTracks)
	for i := range numBlocks {
		trackIdx := i % numTracks
		trackID := fmt.Sprintf("track_%d", trackIdx)
		block := &Block{
			ID:    fmt.Sprintf("block_%d_%d", trackIdx, len(blocksByTrack[trackIdx])),
			Type:  "media",
			Track: trackID,
			Name:  fmt.Sprintf("Block %d-%d", trackIdx, len(blocksByTrack[trackIdx])),
		}
		show.Blocks = append(show.Blocks, block)
		blocksByTrack[trackIdx] = append(blocksByTrack[trackIdx], block)
	}

	for trackIdx := range numTracks {
		blocks := blocksByTrack[trackIdx]
		for i := 1; i < len(blocks); i++ {
			show.Triggers = append(show.Triggers, &Trigger{
				Source:  TriggerSource{Block: blocks[i-1].ID, Signal: "END"},
				Targets: []TriggerTarget{{Block: blocks[i].ID, Hook: "START"}},
			})
		}
	}

	headPerTrack := make([]int, numTracks)
	for i := range numCues {
		cue := show.Blocks[i]
		targets := []TriggerTarget{}
		for trackIdx := range numTracks {
			if headPerTrack[trackIdx] >= len(blocksByTrack[trackIdx]) {
				continue
			}
			block := blocksByTrack[trackIdx][headPerTrack[trackIdx]]
			targets = append(targets, TriggerTarget{Block: block.ID, Hook: "START"})
			depth := len(blocksByTrack[trackIdx]) - headPerTrack[trackIdx]
			advance := max(depth/(numCues-i), 1)
			headPerTrack[trackIdx] += advance
		}
		if len(targets) > 0 {
			show.Triggers = append(show.Triggers, &Trigger{
				Source:  TriggerSource{Block: cue.ID, Signal: "GO"},
				Targets: targets,
			})
		}
	}

	return show
}
