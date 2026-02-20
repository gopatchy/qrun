package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestBuildTimelineFromFixture(t *testing.T) {
	raw, err := os.ReadFile("static/show.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var show Show
	if err := json.Unmarshal(raw, &show); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	timeline, err := BuildTimeline(show)
	if err != nil {
		t.Fatalf("build timeline: %v", err)
	}
	if len(timeline.Tracks) != len(show.Tracks)+1 {
		t.Fatalf("track count mismatch: got %d want %d", len(timeline.Tracks), len(show.Tracks)+1)
	}
	if len(timeline.Rows) == 0 {
		t.Fatalf("expected timeline rows")
	}

	foundCueSignal := false
	for _, row := range timeline.Rows {
		for _, cell := range row.Cells {
			if cell.Event != "GO" || cell.BlockID == "" {
				continue
			}
			block, ok := timeline.Blocks[cell.BlockID]
			if ok && block.Type == "cue" {
				foundCueSignal = true
				break
			}
		}
		if foundCueSignal {
			break
		}
	}
	if !foundCueSignal {
		t.Fatalf("expected at least one cue cell represented as block_id + GO event")
	}
}

func TestBuildTimelineMergesSameBlockEndCell(t *testing.T) {
	show := Show{
		Tracks: []Track{
			{ID: "track1", Name: "Track 1"},
		},
		Blocks: []Block{
			{ID: "cue1", Type: "cue", Name: "Q1"},
			{ID: "a", Type: "light", Track: "track1", Name: "A"},
		},
		Triggers: []Trigger{
			{
				Source: TriggerSource{Block: "cue1", Signal: "GO"},
				Targets: []TriggerTarget{
					{Block: "a", Hook: "START"},
				},
			},
			{
				Source: TriggerSource{Block: "a", Signal: "END"},
				Targets: []TriggerTarget{
					{Block: "a", Hook: "END"},
				},
			},
		},
	}

	timeline, err := BuildTimeline(show)
	if err != nil {
		t.Fatalf("build timeline: %v", err)
	}

	found := false
	for _, row := range timeline.Rows {
		for _, cell := range row.Cells {
			if cell.BlockID == "a" && cell.Event == "END" {
				if !cell.IsSignal || !cell.IsEnd {
					t.Fatalf("expected END cell to include signal+end markers, got signal=%v is_end=%v", cell.IsSignal, cell.IsEnd)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("did not find END cell for block a")
	}
}
