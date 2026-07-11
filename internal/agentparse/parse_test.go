package agentparse

import (
	"testing"
)

func TestClaudeToolInput(t *testing.T) {
	line := []byte(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"pytest -q"},"tool_response":{"is_error":false}}`)
	evs := ExtractCommands(line, "claude-code")
	if len(evs) != 1 || evs[0].Command != "pytest -q" {
		t.Fatalf("%+v", evs)
	}
	if evs[0].ExitCode == nil || *evs[0].ExitCode != 0 {
		t.Fatalf("exit %+v", evs[0].ExitCode)
	}
}

func TestCodexInput(t *testing.T) {
	line := []byte(`{"type":"exec","input":{"command":"go test ./...","workdir":"/tmp/x"},"exit_code":1}`)
	evs := ExtractCommands(line, "codex")
	if len(evs) != 1 || evs[0].Command != "go test ./..." {
		t.Fatalf("%+v", evs)
	}
	if evs[0].ExitCode == nil || *evs[0].ExitCode != 1 {
		t.Fatalf("exit")
	}
}

func TestClaudeContentArray(t *testing.T) {
	line := []byte(`{"message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls -la"}}]}}`)
	evs := ExtractCommands(line, "claude-code")
	if len(evs) != 1 || evs[0].Command != "ls -la" {
		t.Fatalf("%+v", evs)
	}
}
