package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestBuildTimelineFromShowJSON(t *testing.T) {
	buf, err := os.ReadFile("static/show.json")
	if err != nil {
		t.Fatal(err)
	}
	var show Show
	if err := json.Unmarshal(buf, &show); err != nil {
		t.Fatal(err)
	}
	if err := show.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildTimeline(&show); err != nil {
		t.Fatal(err)
	}
}

func TestBuildTimelineFromMockShow(t *testing.T) {
	show := GenerateMockShow(7, 100, 1000)
	if err := show.Validate(); err != nil {
		t.Fatalf("generated show failed validation: %v", err)
	}
	tl, err := BuildTimeline(show)
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}
	if len(tl.Tracks) != 8 {
		t.Errorf("expected 8 tracks (7 + cue), got %d", len(tl.Tracks))
	}
	if len(tl.Blocks) != 1100 {
		t.Errorf("expected 1100 blocks (100 cues + 1000), got %d", len(tl.Blocks))
	}
}

func BenchmarkBuildTimeline(b *testing.B) {
	show := GenerateMockShow(7, 100, 1000)
	if err := show.Validate(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		BuildTimeline(show)
	}
}
