package event

import (
	"testing"
	"time"
)

func TestParseAndValidateShell(t *testing.T) {
	raw := []byte(`{
		"schema": "orma.event.v1",
		"ts": "2026-07-11T18:22:01Z",
		"actor": "human",
		"kind": "shell",
		"source": "shell",
		"command": "docker compose up -d db",
		"cwd": "/home/dev/payments",
		"exit_code": 0
	}`)
	ev, err := ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	ev.Normalize()
	if err := ev.Validate(); err != nil {
		t.Fatal(err)
	}
	if ev.Outcome != "success" {
		t.Fatalf("outcome=%q", ev.Outcome)
	}
	if ev.TS.Year() != 2026 {
		t.Fatalf("ts=%v", ev.TS)
	}
}

func TestAgentRequiresAgentField(t *testing.T) {
	ev := &Event{
		Schema: SchemaV1,
		TS:     time.Now().UTC(),
		Actor:  "agent",
		Kind:   "bash",
		Source: "claude-code",
	}
	cmd := "pytest"
	ev.Command = &cmd
	ev.Normalize()
	if err := ev.Validate(); err == nil {
		t.Fatal("expected error for missing agent")
	}
	ev.Agent = "claude-code"
	if err := ev.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBadSchema(t *testing.T) {
	ev := &Event{
		Schema: "nope",
		TS:     time.Now().UTC(),
		Actor:  "human",
		Kind:   "note",
		Source: "cli",
	}
	if err := ev.Validate(); err == nil {
		t.Fatal("expected schema error")
	}
}
