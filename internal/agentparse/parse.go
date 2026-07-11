package agentparse

import (
	"encoding/json"
	"strings"
	"time"
)

// CommandEvent is a normalized bash-like tool call extracted from agent logs.
type CommandEvent struct {
	Command   string
	CWD       string
	ExitCode  *int
	SessionID string
	ParentID  string
	Tool      string
	TS        time.Time
	Agent     string
	Goal      string
}

// ExtractCommands pulls commands from one JSON object (one JSONL line or blob).
func ExtractCommands(line []byte, defaultAgent string) []CommandEvent {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil
	}
	agent := defaultAgent
	if agent == "" {
		agent = detectAgent(raw)
	}
	sid := firstString(raw, "session_id", "sessionId", "id")
	parent := firstString(raw, "parent_session_id", "parent_id", "parentId")
	ts := parseTS(raw)
	goal := firstString(raw, "goal", "prompt", "user_message")

	var out []CommandEvent

	// Direct fields
	if c := firstString(raw, "command", "cmd"); c != "" {
		out = append(out, CommandEvent{
			Command: c, SessionID: sid, ParentID: parent, TS: ts, Agent: agent, Goal: goal,
			ExitCode: exitFrom(raw), Tool: firstString(raw, "tool", "tool_name"),
			CWD: firstString(raw, "cwd", "workdir", "working_directory"),
		})
	}

	// Claude / tool_use style
	if ti, ok := raw["tool_input"].(map[string]any); ok {
		if c, ok := ti["command"].(string); ok && c != "" {
			ec := exitFrom(raw)
			if tr, ok := raw["tool_response"].(map[string]any); ok {
				if isErr, ok := tr["is_error"].(bool); ok {
					v := 0
					if isErr {
						v = 1
					}
					ec = &v
				}
			}
			out = append(out, CommandEvent{
				Command: c, SessionID: sid, ParentID: parent, TS: ts, Agent: agent,
				ExitCode: ec, Tool: firstString(raw, "tool_name", "tool"),
				CWD: strField(ti, "cwd"), Goal: goal,
			})
		}
	}

	// Codex / nested input
	if ti, ok := raw["input"].(map[string]any); ok {
		if c, ok := ti["command"].(string); ok && c != "" {
			out = append(out, CommandEvent{
				Command: c, SessionID: sid, ParentID: parent, TS: ts, Agent: agent,
				ExitCode: exitFrom(raw), Tool: firstString(raw, "type", "tool"),
				CWD: strField(ti, "workdir"), Goal: goal,
			})
		}
	}

	// OpenAI function call arguments as JSON string
	if args, ok := raw["arguments"].(string); ok && args != "" {
		var m map[string]any
		if json.Unmarshal([]byte(args), &m) == nil {
			if c, ok := m["command"].(string); ok && c != "" {
				out = append(out, CommandEvent{
					Command: c, SessionID: sid, ParentID: parent, TS: ts, Agent: agent,
					ExitCode: exitFrom(raw), Goal: goal,
				})
			}
		}
	}

	// Nested message / content arrays (Claude transcript style)
	out = append(out, walkContent(raw, sid, parent, ts, agent, goal)...)

	// payload / data wrappers (avoid re-walking message which walkContent already handled)
	for _, key := range []string{"payload", "data", "record"} {
		if nested, ok := raw[key].(map[string]any); ok {
			b, _ := json.Marshal(nested)
			for _, e := range ExtractCommands(b, agent) {
				if e.SessionID == "" {
					e.SessionID = sid
				}
				out = append(out, e)
			}
		}
	}

	return dedupe(out)
}

func walkContent(raw map[string]any, sid, parent string, ts time.Time, agent, goal string) []CommandEvent {
	var out []CommandEvent
	msg, _ := raw["message"].(map[string]any)
	if msg == nil {
		msg = raw
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return nil
	}
	for _, item := range content {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		name, _ := m["name"].(string)
		if typ == "tool_use" && (name == "Bash" || name == "bash" || name == "Shell" || name == "shell") {
			if input, ok := m["input"].(map[string]any); ok {
				if c, ok := input["command"].(string); ok && c != "" {
					out = append(out, CommandEvent{
						Command: c, SessionID: sid, ParentID: parent, TS: ts, Agent: agent,
						Tool: name, CWD: strField(input, "cwd"), Goal: goal,
					})
				}
			}
		}
	}
	return out
}

func detectAgent(raw map[string]any) string {
	s := strings.ToLower(firstString(raw, "source", "agent", "app"))
	switch {
	case strings.Contains(s, "claude"):
		return "claude-code"
	case strings.Contains(s, "codex"):
		return "codex"
	case strings.Contains(s, "opencode"):
		return "opencode"
	default:
		return "generic"
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func strField(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func exitFrom(m map[string]any) *int {
	for _, k := range []string{"exit_code", "exitCode", "exit", "status"} {
		switch v := m[k].(type) {
		case float64:
			i := int(v)
			return &i
		case int:
			return &v
		case bool:
			i := 0
			if v {
				i = 1
			}
			return &i
		}
	}
	return nil
}

func parseTS(m map[string]any) time.Time {
	for _, k := range []string{"ts", "timestamp", "created_at", "time"} {
		switch v := m[k].(type) {
		case string:
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				return t
			}
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t
			}
		case float64:
			if v > 1e12 {
				return time.UnixMilli(int64(v))
			}
			return time.Unix(int64(v), 0)
		}
	}
	return time.Now().UTC()
}

func dedupe(in []CommandEvent) []CommandEvent {
	seen := map[string]struct{}{}
	var out []CommandEvent
	for _, e := range in {
		e.Command = strings.TrimSpace(e.Command)
		if e.Command == "" {
			continue
		}
		// same command in one parse batch is one event
		key := e.SessionID + "|" + e.Command + "|" + e.Tool
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if e.TS.IsZero() {
			e.TS = time.Now().UTC()
		}
		out = append(out, e)
	}
	return out
}
