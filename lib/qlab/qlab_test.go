package qlab

import (
	"encoding/json"
	"testing"
	"time"
)

func setupTest(t *testing.T) (*MockServer, *Client) {
	t.Helper()
	mock, err := NewMockServer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mock.Close() })

	client, err := Dial("127.0.0.1", mock.Port())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })

	return mock, client
}

func TestVersion(t *testing.T) {
	mock, client := setupTest(t)
	mock.Version = "5.2.3"

	v, err := client.Version()
	if err != nil {
		t.Fatal(err)
	}
	if v != "5.2.3" {
		t.Errorf("got %q, want %q", v, "5.2.3")
	}
}

func TestWorkspaces(t *testing.T) {
	mock, client := setupTest(t)
	mock.Workspaces = []Workspace{
		{DisplayName: "Show 1", UniqueID: "ws-1"},
		{DisplayName: "Show 2", UniqueID: "ws-2", HasPasscode: true},
	}

	ws, err := client.Workspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 {
		t.Fatalf("got %d workspaces, want 2", len(ws))
	}
	if ws[0].DisplayName != "Show 1" {
		t.Errorf("got %q, want %q", ws[0].DisplayName, "Show 1")
	}
	if ws[0].UniqueID != "ws-1" {
		t.Errorf("got %q, want %q", ws[0].UniqueID, "ws-1")
	}
	if !ws[1].HasPasscode {
		t.Error("expected HasPasscode to be true")
	}
}

func TestConnect(t *testing.T) {
	_, client := setupTest(t)

	if err := client.Connect("ws-1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestConnectWithPasscode(t *testing.T) {
	_, client := setupTest(t)

	if err := client.Connect("ws-1", "secret"); err != nil {
		t.Fatal(err)
	}
}

func TestCueLists(t *testing.T) {
	mock, client := setupTest(t)
	mock.CueLists["ws-1"] = []Cue{
		{
			UniqueID: "list-1",
			Name:     "Main Cue List",
			Type:     "Cue List",
			Cues: []Cue{
				{UniqueID: "cue-1", Number: "1", Name: "Lights Up", Type: "Light"},
				{UniqueID: "cue-2", Number: "2", Name: "Sound", Type: "Audio"},
			},
		},
	}

	lists, err := client.CueLists("ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 {
		t.Fatalf("got %d lists, want 1", len(lists))
	}
	if lists[0].Name != "Main Cue List" {
		t.Errorf("got %q, want %q", lists[0].Name, "Main Cue List")
	}
	if len(lists[0].Cues) != 2 {
		t.Fatalf("got %d cues, want 2", len(lists[0].Cues))
	}
	if lists[0].Cues[0].Name != "Lights Up" {
		t.Errorf("got %q, want %q", lists[0].Cues[0].Name, "Lights Up")
	}
}

func TestCueGet(t *testing.T) {
	mock, client := setupTest(t)
	mock.CueLists["ws-1"] = []Cue{
		{
			UniqueID: "list-1",
			Type:     "Cue List",
			Cues: []Cue{
				{UniqueID: "cue-1", Number: "1", Name: "Lights Up"},
			},
		},
	}

	reply, err := client.CueGet("ws-1", "cue-1", "name")
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := json.Unmarshal(reply.Data, &name); err != nil {
		t.Fatal(err)
	}
	if name != "Lights Up" {
		t.Errorf("got %q, want %q", name, "Lights Up")
	}
}

func TestCueGetByNumber(t *testing.T) {
	mock, client := setupTest(t)
	mock.CueLists["ws-1"] = []Cue{
		{
			UniqueID: "list-1",
			Type:     "Cue List",
			Cues: []Cue{
				{UniqueID: "cue-1", Number: "1", Name: "Lights Up"},
				{UniqueID: "cue-2", Number: "2", Name: "Sound"},
			},
		},
	}

	reply, err := client.CueGetByNumber("ws-1", "2", "name")
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := json.Unmarshal(reply.Data, &name); err != nil {
		t.Fatal(err)
	}
	if name != "Sound" {
		t.Errorf("got %q, want %q", name, "Sound")
	}
}

func TestCueGetNotFound(t *testing.T) {
	mock, client := setupTest(t)
	mock.CueLists["ws-1"] = []Cue{}

	_, err := client.CueGet("ws-1", "nonexistent", "name")
	if err == nil {
		t.Fatal("expected error for nonexistent cue")
	}
}

func TestCueSet(t *testing.T) {
	mock, client := setupTest(t)
	mock.CueLists["ws-1"] = []Cue{
		{
			UniqueID: "list-1",
			Type:     "Cue List",
			Cues: []Cue{
				{UniqueID: "cue-1", Number: "1", Name: "Lights Up"},
			},
		},
	}

	if err := client.CueSet("ws-1", "cue-1", "name", "Blackout"); err != nil {
		t.Fatal(err)
	}

	reply, err := client.CueGet("ws-1", "cue-1", "name")
	if err != nil {
		t.Fatal(err)
	}
	var name string
	json.Unmarshal(reply.Data, &name)
	if name != "Blackout" {
		t.Errorf("got %q, want %q", name, "Blackout")
	}
}

func TestCueSetByNumber(t *testing.T) {
	mock, client := setupTest(t)
	mock.CueLists["ws-1"] = []Cue{
		{
			UniqueID: "list-1",
			Type:     "Cue List",
			Cues: []Cue{
				{UniqueID: "cue-1", Number: "1", Name: "Lights Up"},
			},
		},
	}

	if err := client.CueSetByNumber("ws-1", "1", "name", "Blackout"); err != nil {
		t.Fatal(err)
	}

	reply, err := client.CueGetByNumber("ws-1", "1", "name")
	if err != nil {
		t.Fatal(err)
	}
	var name string
	json.Unmarshal(reply.Data, &name)
	if name != "Blackout" {
		t.Errorf("got %q, want %q", name, "Blackout")
	}
}

func TestUpdates(t *testing.T) {
	mock, client := setupTest(t)

	// Ensure connection is fully established
	_, err := client.Version()
	if err != nil {
		t.Fatal(err)
	}

	mock.SendUpdate("/update/workspace/ws-1/cue_id/cue-1")

	select {
	case u := <-client.Updates():
		if u.Address != "/update/workspace/ws-1/cue_id/cue-1" {
			t.Errorf("got %q, want %q", u.Address, "/update/workspace/ws-1/cue_id/cue-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for update")
	}
}

func TestTransport(t *testing.T) {
	_, client := setupTest(t)

	for _, fn := range []func(string) error{
		client.Go,
		client.Stop,
		client.Pause,
		client.Resume,
		client.Panic,
		client.Reset,
	} {
		if err := fn("ws-1"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSelectedCues(t *testing.T) {
	_, client := setupTest(t)

	cues, err := client.SelectedCues("ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 0 {
		t.Errorf("got %d cues, want 0", len(cues))
	}
}

func TestRunningCues(t *testing.T) {
	_, client := setupTest(t)

	cues, err := client.RunningCues("ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 0 {
		t.Errorf("got %d cues, want 0", len(cues))
	}
}

func TestNestedCueGet(t *testing.T) {
	mock, client := setupTest(t)
	mock.CueLists["ws-1"] = []Cue{
		{
			UniqueID: "list-1",
			Type:     "Cue List",
			Cues: []Cue{
				{
					UniqueID: "group-1",
					Number:   "10",
					Name:     "Group",
					Type:     "Group",
					Cues: []Cue{
						{UniqueID: "nested-1", Number: "10.1", Name: "Nested Cue"},
					},
				},
			},
		},
	}

	reply, err := client.CueGet("ws-1", "nested-1", "name")
	if err != nil {
		t.Fatal(err)
	}
	var name string
	json.Unmarshal(reply.Data, &name)
	if name != "Nested Cue" {
		t.Errorf("got %q, want %q", name, "Nested Cue")
	}
}
