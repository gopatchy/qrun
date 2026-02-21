package main

import (
	"testing"
	"time"
)

func TestBuildTimelineFromMockShow(t *testing.T) {
	t0 := time.Now()
	show := GenerateMockShow(5, 100, 1000)
	t.Logf("GenerateMockShow: %v (%d blocks, %d triggers)", time.Since(t0), len(show.Blocks), len(show.Triggers))

	t1 := time.Now()
	if err := show.Validate(); err != nil {
		t.Fatalf("generated show failed validation: %v", err)
	}
	t.Logf("Validate: %v", time.Since(t1))

	t2 := time.Now()
	tl, err := BuildTimeline(show)
	t.Logf("BuildTimeline: %v", time.Since(t2))
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}
	t.Logf("tracks=%d blocks=%d", len(tl.Tracks), len(tl.Blocks))
}

func BenchmarkBuildTimeline(b *testing.B) {
	show := GenerateMockShow(5, 100, 1000)
	if err := show.Validate(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		BuildTimeline(show)
	}
}
