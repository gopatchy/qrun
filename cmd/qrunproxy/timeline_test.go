package main

import (
	"testing"
	"time"
)

func TestBuildTimelineFromMockShow(t *testing.T) {
	t0 := time.Now()
	show := GenerateMockShow(5, 20, 4, 5)
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
	show := GenerateMockShow(5, 20, 4, 5)
	if err := show.Validate(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		BuildTimeline(show)
	}
}

func TestTimelineOrderDependency(t *testing.T) {
	show := &Show{
		Tracks: []*Track{
			{ID: "T1", Name: "Track 1"},
			{ID: "T2", Name: "Track 2"},
		},
		Blocks: []*Block{
			{ID: "B", Type: "media", Track: "T1", Name: "Block B"},
			{ID: "A", Type: "media", Track: "T1", Name: "Block A"},
			{ID: "C", Type: "media", Track: "T2", Name: "Block C"},
			{ID: "C1", Type: "cue", Name: "Cue 1"},
		},
		Triggers: []*Trigger{
			{
				Source: TriggerSource{Block: "C1", Signal: "GO"},
				Targets: []TriggerTarget{{Block: "A", Hook: "START"}},
			},
			{
				Source: TriggerSource{Block: "A", Signal: "END"},
				Targets: []TriggerTarget{{Block: "C", Hook: "START"}},
			},
			{
				Source: TriggerSource{Block: "C", Signal: "END"},
				Targets: []TriggerTarget{{Block: "B", Hook: "START"}},
			},
		},
	}

	if err := show.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	_, err := BuildTimeline(show)
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}
}
