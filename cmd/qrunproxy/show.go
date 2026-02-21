package main

import (
	"encoding/json"
	"fmt"
)

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

type TriggerSource struct {
	Block  string `json:"block"`
	Signal string `json:"signal"`
}

type TriggerTarget struct {
	Block string `json:"block"`
	Hook  string `json:"hook"`
}

func loadMockShow() (*Show, error) {
	buf, err := staticFS.ReadFile("static/show.json")
	if err != nil {
		return nil, err
	}
	var show Show
	if err := json.Unmarshal(buf, &show); err != nil {
		return nil, err
	}
	return &show, nil
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

func (show *Show) validate() error {
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

	startTargeted := map[string]bool{}
	for _, trigger := range show.Triggers {
		sourceBlock := blocksByID[trigger.Source.Block]
		if sourceBlock == nil {
			return fmt.Errorf("trigger source block %q not found", trigger.Source.Block)
		}
		if !isValidEventForBlock(sourceBlock, trigger.Source.Signal) {
			return fmt.Errorf("trigger source signal %q is invalid for block %q", trigger.Source.Signal, trigger.Source.Block)
		}

		for _, target := range trigger.Targets {
			targetBlock := blocksByID[target.Block]
			if targetBlock == nil {
				return fmt.Errorf("trigger target block %q not found", target.Block)
			}
			if !isValidEventForBlock(targetBlock, target.Hook) {
				return fmt.Errorf("trigger target hook %q is invalid for block %q", target.Hook, target.Block)
			}
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
