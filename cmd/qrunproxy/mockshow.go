package main

import (
	"fmt"
	"math/rand/v2"
)

var trackNamePool = []string{
	"Lighting", "Fill Light", "Spots", "Video", "Video OVL",
	"Audio", "SFX", "Ambience", "Pyro", "Fog", "Motors",
	"Follow Spot", "Haze", "Projector", "LED Wall",
}

var lightNamePool = []string{
	"Wash", "Focus", "Spot", "Amber", "Blue", "Cool", "Warm",
	"Flood", "Strobe", "Blackout", "Dim", "Bright", "Sunrise",
}

var mediaNamePool = []string{
	"Loop", "Projection", "Background", "Overlay", "Flash",
	"Ambience", "Underscore", "Sting", "Bumper", "Transition",
}

var delayNamePool = []string{
	"1s Delay", "2s Delay", "3s Delay", "5s Delay", "Hold",
}

func GenerateMockShow(numTracks, numCues, numBlocks int) *Show {
	rng := rand.New(rand.NewPCG(42, 0))

	show := &Show{}
	blockIdx := 0
	curCueName := ""
	nextBlockID := func(trackIdx int) string {
		id := fmt.Sprintf("%s-t%d-b%d", curCueName, trackIdx, blockIdx)
		blockIdx++
		return id
	}

	names := make([]string, len(trackNamePool))
	copy(names, trackNamePool)
	rng.Shuffle(len(names), func(i, j int) {
		names[i], names[j] = names[j], names[i]
	})
	for i := range numTracks {
		name := names[i%len(names)]
		if i >= len(names) {
			name = fmt.Sprintf("%s %d", name, i/len(names)+1)
		}
		show.Tracks = append(show.Tracks, &Track{
			ID:   fmt.Sprintf("track_%d", i),
			Name: name,
		})
	}

	randBlock := func(trackIdx int) *Block {
		r := rng.Float64()
		var typ, name string
		var loop bool
		switch {
		case r < 0.30:
			typ, name = "light", lightNamePool[rng.IntN(len(lightNamePool))]
		case r < 0.55:
			typ, name = "media", mediaNamePool[rng.IntN(len(mediaNamePool))]
		case r < 0.70:
			typ, name, loop = "media", mediaNamePool[rng.IntN(len(mediaNamePool))], true
		case r < 0.80:
			typ, name = "delay", delayNamePool[rng.IntN(len(delayNamePool))]
		default:
			typ, name = "light", lightNamePool[rng.IntN(len(lightNamePool))]
		}
		return &Block{
			ID:    nextBlockID(trackIdx),
			Type:  typ,
			Track: fmt.Sprintf("track_%d", trackIdx),
			Name:  name,
			Loop:  loop,
		}
	}

	placed := 0
	cueIdx := 0
	scene := 0
	chainFromByTrack := make(map[int]*Block)
	needsEnd := make(map[string]*Block)
	allowedTracks := make(map[int]bool, numTracks)
	for i := range numTracks {
		allowedTracks[i] = true
	}

	for placed < numBlocks && cueIdx < numCues {
		scene++
		cuesInScene := 2 + rng.IntN(3)

		for intra := 1; intra <= cuesInScene; intra++ {
			if placed >= numBlocks || cueIdx >= numCues {
				break
			}
			for trackIdx, blk := range chainFromByTrack {
				if needsEnd[blk.ID] == nil {
					delete(chainFromByTrack, trackIdx)
				}
			}

			curCueName = fmt.Sprintf("S%d Q%d", scene, intra)
			cue := &Block{
				ID:   curCueName,
				Type: "cue",
				Name: curCueName,
			}
			show.Blocks = append(show.Blocks, cue)
			cueIdx++

			blocksThisCue := 1 + rng.IntN(numTracks*2)
			cueTargets := []TriggerTarget{}
			for id, blk := range needsEnd {
				cueTargets = append(cueTargets, TriggerTarget{Block: blk.ID, Hook: "END"})
				delete(needsEnd, id)
				for ti := range numTracks {
					if chainFromByTrack[ti] == blk {
						allowedTracks[ti] = true
					}
				}
			}
			for range blocksThisCue {
				if placed >= numBlocks {
					break
				}
				trackIdx := rng.IntN(numTracks)
				if !allowedTracks[trackIdx] {
					continue
				}
				block := randBlock(trackIdx)
				show.Blocks = append(show.Blocks, block)
				placed++
				if prev := chainFromByTrack[trackIdx]; prev != nil {
					show.Triggers = append(show.Triggers, &Trigger{
						Source:  TriggerSource{Block: prev.ID, Signal: "END"},
						Targets: []TriggerTarget{{Block: block.ID, Hook: "START"}},
					})
					delete(needsEnd, prev.ID)
				} else {
					cueTargets = append(cueTargets, TriggerTarget{Block: block.ID, Hook: "START"})
				}
				if !block.hasDefinedTiming() {
					needsEnd[block.ID] = block
					allowedTracks[trackIdx] = false
				}
				chainFromByTrack[trackIdx] = block
			}

			if len(cueTargets) > 0 {
				show.Triggers = append(show.Triggers, &Trigger{
					Source:  TriggerSource{Block: cue.ID, Signal: "GO"},
					Targets: cueTargets,
				})
			}
		}

		endTargets := []TriggerTarget{}
		for id, blk := range needsEnd {
			endTargets = append(endTargets, TriggerTarget{Block: blk.ID, Hook: "END"})
			delete(needsEnd, id)
			for ti := range numTracks {
				if chainFromByTrack[ti] == blk {
					allowedTracks[ti] = true
				}
			}
		}
		if len(endTargets) > 0 {
			endCueName := fmt.Sprintf("S%d End", scene)
			endCue := &Block{
				ID:   endCueName,
				Type: "cue",
				Name: endCueName,
			}
			show.Blocks = append(show.Blocks, endCue)
			cueIdx++
			show.Triggers = append(show.Triggers, &Trigger{
				Source:  TriggerSource{Block: endCue.ID, Signal: "GO"},
				Targets: endTargets,
			})
		}
	}

	for cueIdx < numCues {
		scene++
		emptyCueName := fmt.Sprintf("S%d Q1", scene)
		cue := &Block{
			ID:   emptyCueName,
			Type: "cue",
			Name: emptyCueName,
		}
		show.Blocks = append(show.Blocks, cue)
		cueIdx++
	}

	return show
}
