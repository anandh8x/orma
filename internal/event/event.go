package event

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const SchemaV1 = "orma.event.v1"

// Event is the canonical activity record (orma.event.v1).
type Event struct {
	Schema             string         `json:"schema"`
	TS                 time.Time      `json:"ts"`
	Actor              string         `json:"actor"`
	Kind               string         `json:"kind"`
	Source             string         `json:"source"`
	Agent              string         `json:"agent,omitempty"`
	EventID            string         `json:"event_id,omitempty"`
	SessionID          string         `json:"session_id,omitempty"`
	ParentSessionID    string         `json:"parent_session_id,omitempty"`
	Command            *string        `json:"command"`
	CWD                string         `json:"cwd,omitempty"`
	ProjectRoot        string         `json:"project_root,omitempty"`
	ExitCode           *int           `json:"exit_code"`
	DurationMS         *int           `json:"duration_ms,omitempty"`
	Outcome            string         `json:"outcome,omitempty"`
	StderrExcerpt      string         `json:"stderr_excerpt,omitempty"`
	StderrFingerprint  string         `json:"stderr_fingerprint,omitempty"`
	StdoutFingerprint  string         `json:"stdout_fingerprint,omitempty"`
	Tool               string         `json:"tool,omitempty"`
	Goal               string         `json:"goal,omitempty"`
	Host               string         `json:"host,omitempty"`
	Shell              string         `json:"shell,omitempty"`
	Tags               []string       `json:"tags,omitempty"`
	RawRef             string         `json:"raw_ref,omitempty"`
	Meta               map[string]any `json:"meta,omitempty"`
	Text               string         `json:"text,omitempty"`
}

// ParseJSON decodes one event object.
func ParseJSON(data []byte) (*Event, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	ev := &Event{}
	// Flexible timestamp
	if v, ok := raw["ts"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, fmt.Errorf("ts: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t, err = time.Parse(time.RFC3339, s)
			if err != nil {
				return nil, fmt.Errorf("ts parse: %w", err)
			}
		}
		ev.TS = t.UTC()
	}

	_ = json.Unmarshal(raw["schema"], &ev.Schema)
	_ = json.Unmarshal(raw["actor"], &ev.Actor)
	_ = json.Unmarshal(raw["kind"], &ev.Kind)
	_ = json.Unmarshal(raw["source"], &ev.Source)
	_ = json.Unmarshal(raw["agent"], &ev.Agent)
	_ = json.Unmarshal(raw["event_id"], &ev.EventID)
	_ = json.Unmarshal(raw["session_id"], &ev.SessionID)
	_ = json.Unmarshal(raw["parent_session_id"], &ev.ParentSessionID)
	_ = json.Unmarshal(raw["cwd"], &ev.CWD)
	_ = json.Unmarshal(raw["project_root"], &ev.ProjectRoot)
	_ = json.Unmarshal(raw["outcome"], &ev.Outcome)
	_ = json.Unmarshal(raw["stderr_excerpt"], &ev.StderrExcerpt)
	_ = json.Unmarshal(raw["stderr_fingerprint"], &ev.StderrFingerprint)
	_ = json.Unmarshal(raw["stdout_fingerprint"], &ev.StdoutFingerprint)
	_ = json.Unmarshal(raw["tool"], &ev.Tool)
	_ = json.Unmarshal(raw["goal"], &ev.Goal)
	_ = json.Unmarshal(raw["host"], &ev.Host)
	_ = json.Unmarshal(raw["shell"], &ev.Shell)
	_ = json.Unmarshal(raw["raw_ref"], &ev.RawRef)
	_ = json.Unmarshal(raw["text"], &ev.Text)
	_ = json.Unmarshal(raw["tags"], &ev.Tags)

	if v, ok := raw["command"]; ok && string(v) != "null" {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			ev.Command = &s
		}
	}
	if v, ok := raw["exit_code"]; ok && string(v) != "null" {
		var n int
		if err := json.Unmarshal(v, &n); err == nil {
			ev.ExitCode = &n
		}
	}
	if v, ok := raw["duration_ms"]; ok && string(v) != "null" {
		var n int
		if err := json.Unmarshal(v, &n); err == nil {
			ev.DurationMS = &n
		}
	}
	if v, ok := raw["meta"]; ok && string(v) != "null" {
		_ = json.Unmarshal(v, &ev.Meta)
	}

	return ev, nil
}

// Validate checks required fields for orma.event.v1.
func (e *Event) Validate() error {
	if e.Schema != SchemaV1 {
		return fmt.Errorf("schema must be %q", SchemaV1)
	}
	if e.TS.IsZero() {
		return fmt.Errorf("ts is required")
	}
	switch e.Actor {
	case "human", "agent":
	default:
		return fmt.Errorf("actor must be human or agent")
	}
	if strings.TrimSpace(e.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(e.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if e.Actor == "agent" && strings.TrimSpace(e.Agent) == "" {
		return fmt.Errorf("agent is required when actor is agent")
	}
	switch e.Kind {
	case "shell", "bash":
		if e.Command == nil || strings.TrimSpace(*e.Command) == "" {
			return fmt.Errorf("command is required for kind %s", e.Kind)
		}
	case "note", "prompt", "session_mark":
		// command optional
	default:
		// other kinds: command optional
	}
	return nil
}

// Normalize fills derived fields (outcome from exit_code, trim command).
func (e *Event) Normalize() {
	if e.Command != nil {
		s := strings.TrimSpace(*e.Command)
		e.Command = &s
	}
	if e.Outcome == "" && e.ExitCode != nil {
		if *e.ExitCode == 0 {
			e.Outcome = "success"
		} else {
			e.Outcome = "fail"
		}
	}
	if e.Outcome == "" {
		e.Outcome = "unknown"
	}
	e.Actor = strings.TrimSpace(e.Actor)
	e.Kind = strings.TrimSpace(e.Kind)
	e.Source = strings.TrimSpace(e.Source)
	e.Agent = strings.TrimSpace(e.Agent)
}

// TruncateExcerpt limits stderr-like text.
func TruncateExcerpt(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
