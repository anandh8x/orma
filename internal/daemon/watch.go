package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/ingest"
	"github.com/anandh8x/orma/internal/store"
)

func scanJSONLDir(ctx context.Context, st *store.Store, cfg *config.Config, dir string, seen map[string]int64) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	svc := &ingest.Service{Store: st, Config: cfg}

	return filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil || fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") && !strings.HasSuffix(path, ".json") {
			return nil
		}
		// skip huge files
		if fi.Size() > 50*1024*1024 {
			return nil
		}
		mod := fi.ModTime().UnixNano()
		if prev, ok := seen[path]; ok && prev == mod {
			return nil
		}
		// only process recently modified
		if time.Since(fi.ModTime()) > 48*time.Hour && seen[path] != 0 {
			seen[path] = mod
			return nil
		}
		_ = importAgentFile(ctx, svc, path)
		seen[path] = mod
		return nil
	})
}

// ImportAgentFile parses a session json/jsonl file into events.
func ImportAgentFile(ctx context.Context, svc *ingest.Service, path string) error {
	return importAgentFile(ctx, svc, path)
}

func importAgentFile(ctx context.Context, svc *ingest.Service, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 2*1024*1024)

	agent := detectAgent(path)
	sessionID := filepath.Base(path)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		cmd, ok := extractCommand(line)
		if !ok || cmd == "" {
			continue
		}
		ev := map[string]any{
			"schema":     "orma.event.v1",
			"ts":         time.Now().UTC().Format(time.RFC3339Nano),
			"actor":      "agent",
			"agent":      agent,
			"kind":       "bash",
			"source":     agent,
			"command":    cmd,
			"session_id": sessionID,
			"outcome":    "unknown",
		}
		// try exit from line
		if strings.Contains(line, "\"exit_code\"") {
			var raw map[string]any
			if json.Unmarshal([]byte(line), &raw) == nil {
				if v, ok := raw["exit_code"].(float64); ok {
					ev["exit_code"] = int(v)
				}
			}
		}
		b, _ := json.Marshal(ev)
		_, _ = svc.IngestOne(ctx, b)
	}
	return sc.Err()
}

func detectAgent(path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "codex"):
		return "codex"
	case strings.Contains(p, "claude"):
		return "claude-code"
	case strings.Contains(p, "opencode"):
		return "opencode"
	default:
		return "generic"
	}
}

func extractCommand(line string) (string, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return "", false
	}
	// common shapes
	if c, ok := asString(raw["command"]); ok {
		return c, true
	}
	if c, ok := asString(raw["cmd"]); ok {
		return c, true
	}
	if ti, ok := raw["tool_input"].(map[string]any); ok {
		if c, ok := asString(ti["command"]); ok {
			return c, true
		}
	}
	if ti, ok := raw["input"].(map[string]any); ok {
		if c, ok := asString(ti["command"]); ok {
			return c, true
		}
	}
	// nested message content
	if msg, ok := raw["message"].(map[string]any); ok {
		if c, ok := asString(msg["command"]); ok {
			return c, true
		}
	}
	return "", false
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok && s != ""
}
