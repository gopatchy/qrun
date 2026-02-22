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

type chainable struct {
	block         *Block
	trackIdx      int
	sameTrackOnly bool
	fromEnded     bool
}

type mockShowGen struct {
	rng        *rand.Rand
	show       *Show
	numTracks  int
	blockIdx   int
	curCueName string
	triggerIdx map[TriggerSource]*Trigger
	needsEnd   map[int]*Block
	chainFrom  []chainable
}

func GenerateMockShow(numTracks, numScenes, avgCuesPerScene, avgBlocksPerCue int) *Show {
	g := &mockShowGen{
		rng:        rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		show:       &Show{},
		numTracks:  numTracks,
		triggerIdx: map[TriggerSource]*Trigger{},
		needsEnd:   map[int]*Block{},
	}

	g.generateTracks()

	for scene := 1; scene <= numScenes; scene++ {
		cuesInScene := 1 + g.rng.IntN(avgCuesPerScene*2)
		for intra := 1; intra <= cuesInScene; intra++ {
			g.generateCue(fmt.Sprintf("S%d Q%d", scene, intra), avgBlocksPerCue)
		}
		g.generateEndOfScene(scene)
	}

	return g.show
}

func (g *mockShowGen) generateTracks() {
	names := make([]string, len(trackNamePool))
	copy(names, trackNamePool)
	g.rng.Shuffle(len(names), func(i, j int) {
		names[i], names[j] = names[j], names[i]
	})
	for i := range g.numTracks {
		name := names[i%len(names)]
		if i >= len(names) {
			name = fmt.Sprintf("%s %d", name, i/len(names)+1)
		}
		g.show.Tracks = append(g.show.Tracks, &Track{
			ID:   fmt.Sprintf("track_%d", i),
			Name: name,
		})
	}
}

func (g *mockShowGen) nextBlockID(trackIdx int) string {
	id := fmt.Sprintf("%s-t%d-b%d", g.curCueName, trackIdx, g.blockIdx)
	g.blockIdx++
	return id
}

func (g *mockShowGen) randLight() Block {
	return Block{
		Type: "light",
		Name: lightNamePool[g.rng.IntN(len(lightNamePool))],
	}
}

func (g *mockShowGen) randMedia() Block {
	return Block{
		Type: "media",
		Name: mediaNamePool[g.rng.IntN(len(mediaNamePool))],
	}
}

func (g *mockShowGen) randLoopingMedia() Block {
	return Block{
		Type: "media",
		Name: mediaNamePool[g.rng.IntN(len(mediaNamePool))],
		Loop: true,
	}
}

func (g *mockShowGen) randDelay() Block {
	return Block{
		Type: "delay",
		Name: delayNamePool[g.rng.IntN(len(delayNamePool))],
	}
}

func (g *mockShowGen) randBlock(trackIdx int) *Block {
	r := g.rng.Float64()
	var b Block
	switch {
	case r < 0.50:
		b = g.randLight()
	case r < 0.75:
		b = g.randMedia()
	case r < 0.90:
		b = g.randLoopingMedia()
	default:
		b = g.randDelay()
	}
	b.ID = g.nextBlockID(trackIdx)
	b.Track = fmt.Sprintf("track_%d", trackIdx)
	return &b
}

func (g *mockShowGen) addTrigger(source TriggerSource, target TriggerTarget) {
	if t := g.triggerIdx[source]; t != nil {
		t.Targets = append(t.Targets, target)
		return
	}
	t := &Trigger{Source: source, Targets: []TriggerTarget{target}}
	g.show.Triggers = append(g.show.Triggers, t)
	g.triggerIdx[source] = t
}

func (g *mockShowGen) endPreviousBlocks() []TriggerTarget {
	var cueTargets []TriggerTarget
	for trackIdx, blk := range g.needsEnd {
		hook := "END"
		if g.rng.Float64() < 0.3 {
			hook = "FADE_OUT"
		}
		cueTargets = append(cueTargets, TriggerTarget{Block: blk.ID, Hook: hook})
		g.chainFrom = append(g.chainFrom, chainable{block: blk, trackIdx: trackIdx, sameTrackOnly: true, fromEnded: true})
		delete(g.needsEnd, trackIdx)
	}
	return cueTargets
}

func (g *mockShowGen) chainBlock(block *Block, trackIdx int, cueTargets *[]TriggerTarget) {
	for i, c := range g.chainFrom {
		if c.trackIdx == trackIdx {
			g.addTrigger(
				TriggerSource{Block: c.block.ID, Signal: "END"},
				TriggerTarget{Block: block.ID, Hook: "START"},
			)
			g.chainFrom = append(g.chainFrom[:i], g.chainFrom[i+1:]...)
			return
		}
	}
	if g.rng.Float64() < 0.3 {
		var candidates []int
		for i, c := range g.chainFrom {
			if !c.sameTrackOnly {
				candidates = append(candidates, i)
			}
		}
		if len(candidates) > 0 {
			idx := candidates[g.rng.IntN(len(candidates))]
			c := g.chainFrom[idx]
			g.addTrigger(
				TriggerSource{Block: c.block.ID, Signal: "END"},
				TriggerTarget{Block: block.ID, Hook: "START"},
			)
			g.chainFrom = append(g.chainFrom[:idx], g.chainFrom[idx+1:]...)
			return
		}
	}
	*cueTargets = append(*cueTargets, TriggerTarget{Block: block.ID, Hook: "START"})
}

func (g *mockShowGen) promoteChainFrom() {
	filtered := g.chainFrom[:0]
	for _, c := range g.chainFrom {
		if !c.fromEnded {
			c.sameTrackOnly = false
			filtered = append(filtered, c)
		}
	}
	g.chainFrom = filtered
}

func (g *mockShowGen) generateCue(name string, avgBlocksPerCue int) {
	g.curCueName = name
	cue := &Block{
		ID:   name,
		Type: "cue",
		Name: name,
	}
	g.show.Blocks = append(g.show.Blocks, cue)

	cueTargets := g.endPreviousBlocks()

	blocksThisCue := 1 + g.rng.IntN(avgBlocksPerCue*2)
	for range blocksThisCue {
		trackIdx := g.rng.IntN(g.numTracks)
		if g.needsEnd[trackIdx] != nil {
			continue
		}

		block := g.randBlock(trackIdx)
		g.show.Blocks = append(g.show.Blocks, block)
		g.chainBlock(block, trackIdx, &cueTargets)

		if !block.hasDefinedTiming() {
			g.needsEnd[trackIdx] = block
		} else {
			g.chainFrom = append(g.chainFrom, chainable{block: block, trackIdx: trackIdx, sameTrackOnly: true})
		}
	}

	g.promoteChainFrom()

	if len(cueTargets) > 0 {
		g.show.Triggers = append(g.show.Triggers, &Trigger{
			Source:  TriggerSource{Block: cue.ID, Signal: "GO"},
			Targets: cueTargets,
		})
	}
}

func (g *mockShowGen) generateEndOfScene(scene int) {
	var endTargets []TriggerTarget
	for trackIdx, blk := range g.needsEnd {
		hook := "END"
		if g.rng.Float64() < 0.3 {
			hook = "FADE_OUT"
		}
		endTargets = append(endTargets, TriggerTarget{Block: blk.ID, Hook: hook})
		delete(g.needsEnd, trackIdx)
	}
	g.chainFrom = nil
	if len(endTargets) > 0 {
		endCueName := fmt.Sprintf("S%d End", scene)
		endCue := &Block{
			ID:   endCueName,
			Type: "cue",
			Name: endCueName,
		}
		g.show.Blocks = append(g.show.Blocks, endCue)
		g.show.Triggers = append(g.show.Triggers, &Trigger{
			Source:  TriggerSource{Block: endCue.ID, Signal: "GO"},
			Targets: endTargets,
		})
	}
}
