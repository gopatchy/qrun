package main

import "fmt"

type Show struct {
	Tracks   []*Track   `json:"tracks"`
	Blocks   []*Block   `json:"blocks"`
	Triggers []*Trigger `json:"triggers"`
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

func (t *Trigger) String() string {
	s := fmt.Sprintf("%s/%s ->", t.Source.Block, t.Source.Signal)
	for _, target := range t.Targets {
		s += fmt.Sprintf(" %s/%s", target.Block, target.Hook)
	}
	return s
}

type TriggerSource struct {
	Block  string `json:"block"`
	Signal string `json:"signal"`
}

type TriggerTarget struct {
	Block string `json:"block"`
	Hook  string `json:"hook"`
}

func (block *Block) hasDefinedTiming() bool {
	if block.Type == "cue" || block.Type == "delay" {
		return true
	}
	if block.Type == "media" && !block.Loop {
		return true
	}
	return false
}

func isValidEventForBlock(block *Block, event string) bool {
	if block.Type == "cue" {
		return event == "GO"
	}
	switch event {
	case "START", "FADE_OUT", "END":
		return true
	default:
		return false
	}
}

func (show *Show) Validate() error {
	if show == nil {
		return fmt.Errorf("show is nil")
	}

	trackIDs := map[string]bool{}
	for _, track := range show.Tracks {
		if trackIDs[track.ID] {
			return fmt.Errorf("duplicate track id %q", track.ID)
		}
		trackIDs[track.ID] = true
	}

	blocksByID := map[string]*Block{}
	for _, block := range show.Blocks {
		if blocksByID[block.ID] != nil {
			return fmt.Errorf("duplicate block id %q", block.ID)
		}
		blocksByID[block.ID] = block
		if block.Type == "cue" {
			continue
		}
		if !trackIDs[block.Track] {
			return fmt.Errorf("block %q uses unknown track %q", block.ID, block.Track)
		}
	}

	type blockEvent struct {
		block string
		event string
	}
	hookTargeted := map[blockEvent]bool{}
	startTargeted := map[string]bool{}
	sourceUsed := map[blockEvent]bool{}
	signalTargetedBy := map[blockEvent]*Trigger{}

	for _, trigger := range show.Triggers {
		for _, target := range trigger.Targets {
			signalTargetedBy[blockEvent{target.Block, target.Hook}] = trigger
		}
	}

	for _, trigger := range show.Triggers {
		sourceBlock := blocksByID[trigger.Source.Block]
		if sourceBlock == nil {
			return fmt.Errorf("trigger source block %q not found", trigger.Source.Block)
		}

		targetedTracks := map[string]string{}
		for _, target := range trigger.Targets {
			targetBlock := blocksByID[target.Block]
			if prev, ok := targetedTracks[targetBlock.Track]; ok {
				return fmt.Errorf("trigger conflict: %s targets multiple blocks on track %q (%q and %q)",
					trigger, targetBlock.Track, prev, target.Block)
			}
			targetedTracks[targetBlock.Track] = target.Block
		}

		if t, ok := signalTargetedBy[blockEvent{trigger.Source.Block, trigger.Source.Signal}]; ok {
			sameTrackSingle := len(trigger.Targets) == 1 && blocksByID[trigger.Targets[0].Block].Track == sourceBlock.Track
			if !sameTrackSingle {
				return fmt.Errorf("trigger conflict: %s vs %s", t, trigger)
			}
		}
		if !isValidEventForBlock(sourceBlock, trigger.Source.Signal) {
			return fmt.Errorf("trigger source signal %q is invalid for block %q", trigger.Source.Signal, trigger.Source.Block)
		}
		src := blockEvent{trigger.Source.Block, trigger.Source.Signal}
		if sourceUsed[src] {
			return fmt.Errorf("duplicate trigger source: block %q signal %q", trigger.Source.Block, trigger.Source.Signal)
		}
		sourceUsed[src] = true

		for _, target := range trigger.Targets {
			targetBlock := blocksByID[target.Block]
			if targetBlock == nil {
				return fmt.Errorf("trigger target block %q not found", target.Block)
			}
			if !isValidEventForBlock(targetBlock, target.Hook) {
				return fmt.Errorf("trigger target hook %q is invalid for block %q", target.Hook, target.Block)
			}
			hookTargeted[blockEvent{target.Block, target.Hook}] = true
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
		if !block.hasDefinedTiming() && !hookTargeted[blockEvent{block.ID, "FADE_OUT"}] && !hookTargeted[blockEvent{block.ID, "END"}] {
			return fmt.Errorf("block %q has no defined timing and nothing triggers its FADE_OUT or END", block.ID)
		}
	}

	for _, trigger := range show.Triggers {
		sourceBlock := blocksByID[trigger.Source.Block]
		for _, target := range trigger.Targets {
			targetBlock := blocksByID[target.Block]
			if sourceBlock.Type != "cue" && targetBlock.Type != "cue" && sourceBlock.Track == targetBlock.Track && target.Hook == "START" && trigger.Source.Signal != "END" {
				return fmt.Errorf("same-track START trigger from %q to %q must use END signal, not %s", sourceBlock.ID, targetBlock.ID, trigger.Source.Signal)
			}
		}
		if sourceBlock.hasDefinedTiming() {
			continue
		}
		signal := trigger.Source.Signal
		if signal != "FADE_OUT" && signal != "END" {
			continue
		}
		if signal == "END" && hookTargeted[blockEvent{sourceBlock.ID, "FADE_OUT"}] {
			continue
		}
		if !hookTargeted[blockEvent{sourceBlock.ID, signal}] {
			return fmt.Errorf("block %q has no defined timing and nothing triggers its %s, so its %s signal will never fire", sourceBlock.ID, signal, signal)
		}
	}

	return nil
}
