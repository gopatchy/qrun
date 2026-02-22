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

func GenerateMockShow(numTracks, numScenes, avgCuesPerScene, avgBlocksPerCue int) *Show {
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

	chainFromByTrack := make(map[int]*Block)
	needsEnd := make(map[string]*Block)
	allowedTracks := make(map[int]bool, numTracks)
	for i := range numTracks {
		allowedTracks[i] = true
	}

	for scene := 1; scene <= numScenes; scene++ {
		cuesInScene := 1 + rng.IntN(avgCuesPerScene*2)

		for intra := 1; intra <= cuesInScene; intra++ {
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

			blocksThisCue := 1 + rng.IntN(avgBlocksPerCue*2)
			cueTargets := []TriggerTarget{}
			for id, blk := range needsEnd {
				hook := "END"
				if rng.Float64() < 0.3 {
					hook = "FADE_OUT"
				}
				cueTargets = append(cueTargets, TriggerTarget{Block: blk.ID, Hook: hook})
				delete(needsEnd, id)
				for ti := range numTracks {
					if chainFromByTrack[ti] == blk {
						allowedTracks[ti] = true
					}
				}
			}
			for range blocksThisCue {
				trackIdx := rng.IntN(numTracks)
				if !allowedTracks[trackIdx] {
					continue
				}
				block := randBlock(trackIdx)
				show.Blocks = append(show.Blocks, block)
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
			hook := "END"
			if rng.Float64() < 0.3 {
				hook = "FADE_OUT"
			}
			endTargets = append(endTargets, TriggerTarget{Block: blk.ID, Hook: hook})
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
			show.Triggers = append(show.Triggers, &Trigger{
				Source:  TriggerSource{Block: endCue.ID, Signal: "GO"},
				Targets: endTargets,
			})
		}
	}

	return show
}
