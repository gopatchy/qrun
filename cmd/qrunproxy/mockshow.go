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
	nextBlockID := func() string {
		id := fmt.Sprintf("b%d", blockIdx)
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
			ID:    nextBlockID(),
			Type:  typ,
			Track: fmt.Sprintf("track_%d", trackIdx),
			Name:  name,
			Loop:  loop,
		}
	}

	placed := 0
	cueIdx := 0
	scene := 0

	for placed < numBlocks && cueIdx < numCues {
		scene++
		cuesInScene := 2 + rng.IntN(3)

		for intra := 1; intra <= cuesInScene; intra++ {
			if placed >= numBlocks || cueIdx >= numCues {
				break
			}

			cue := &Block{
				ID:   fmt.Sprintf("q%d", cueIdx),
				Type: "cue",
				Name: fmt.Sprintf("S%d Q%d", scene, intra),
			}
			show.Blocks = append(show.Blocks, cue)
			cueIdx++

			tracksThisCue := numTracks - rng.IntN(2)
			perm := rng.Perm(numTracks)

			cueTargets := []TriggerTarget{}
			for _, trackIdx := range perm[:tracksThisCue] {
				if placed >= numBlocks {
					break
				}
				block := randBlock(trackIdx)
				show.Blocks = append(show.Blocks, block)
				cueTargets = append(cueTargets, TriggerTarget{Block: block.ID, Hook: "START"})
				placed++

				prev := block
				chainLen := rng.IntN(3)
				for range chainLen {
					if placed >= numBlocks {
						break
					}
					if !prev.hasDefinedTiming() {
						break
					}
					next := randBlock(trackIdx)
					show.Blocks = append(show.Blocks, next)
					show.Triggers = append(show.Triggers, &Trigger{
						Source:  TriggerSource{Block: prev.ID, Signal: "END"},
						Targets: []TriggerTarget{{Block: next.ID, Hook: "START"}},
					})
					prev = next
					placed++
				}
			}

			if len(cueTargets) > 0 {
				show.Triggers = append(show.Triggers, &Trigger{
					Source:  TriggerSource{Block: cue.ID, Signal: "GO"},
					Targets: cueTargets,
				})
			}
		}
	}

	for cueIdx < numCues {
		scene++
		cue := &Block{
			ID:   fmt.Sprintf("q%d", cueIdx),
			Type: "cue",
			Name: fmt.Sprintf("S%d Q1", scene),
		}
		show.Blocks = append(show.Blocks, cue)
		cueIdx++
	}

	return show
}
