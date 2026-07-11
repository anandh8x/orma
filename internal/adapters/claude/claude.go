package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Connect writes Claude Code hooks that call orma ingest on Bash tool use.
func Connect(ormaBin string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	hookScript := filepath.Join(dir, "orma-hook.sh")
	script := fmt.Sprintf(`#!/usr/bin/env bash
# Orma Claude Code PostToolUse hook
set -euo pipefail
input=$(cat)
cmd=$(printf '%%s' "$input" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
[[ -z "$cmd" ]] && exit 0
sid=$(printf '%%s' "$input" | jq -r '.session_id // empty' 2>/dev/null || true)
err=$(printf '%%s' "$input" | jq -r '.tool_response.is_error // false' 2>/dev/null || echo false)
ec=0
[[ "$err" == "true" ]] && ec=1
ts=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)
payload=$(jq -nc \
  --arg cmd "$cmd" \
  --arg sid "$sid" \
  --arg ts "$ts" \
  --argjson ec "$ec" \
  '{schema:"orma.event.v1",ts:$ts,actor:"agent",agent:"claude-code",kind:"bash",source:"claude-code",command:$cmd,session_id:$sid,exit_code:$ec}')
printf '%%s' "$payload" | %q ingest --json >/dev/null 2>&1 || true
`, ormaBin)
	if err := os.WriteFile(hookScript, []byte(script), 0o700); err != nil {
		return "", err
	}

	settingsPath := filepath.Join(dir, "orma-settings.snippet.json")
	snippet := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []map[string]any{
				{
					"matcher": "Bash",
					"hooks": []map[string]any{
						{
							"type":    "command",
							"command": hookScript,
							"async":   true,
						},
					},
				},
			},
		},
	}
	b, _ := json.MarshalIndent(snippet, "", "  ")
	if err := os.WriteFile(settingsPath, b, 0o600); err != nil {
		return "", err
	}

	return fmt.Sprintf("wrote %s and %s (merge snippet into Claude Code settings hooks)", hookScript, settingsPath), nil
}
