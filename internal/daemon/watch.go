package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anandh8x/orma/internal/agentparse"
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
		if fi.Size() > 50*1024*1024 {
			return nil
		}
		mod := fi.ModTime().UnixNano()
		if prev, ok := seen[path]; ok && prev == mod {
			return nil
		}
		if time.Since(fi.ModTime()) > 48*time.Hour && seen[path] != 0 {
			seen[path] = mod
			return nil
		}
		_ = ImportAgentFile(ctx, svc, path)
		seen[path] = mod
		return nil
	})
}

// ImportAgentFile parses a session json/jsonl file into events.
func ImportAgentFile(ctx context.Context, svc *ingest.Service, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	agent := detectAgent(path)
	sessionID := filepath.Base(path)
	// strip extension for cleaner session ids
	sessionID = strings.TrimSuffix(sessionID, filepath.Ext(sessionID))

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 4*1024*1024)

	// json array file?
	if strings.HasSuffix(path, ".json") {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// try as single object or array
		var arr []json.RawMessage
		if json.Unmarshal(data, &arr) == nil {
			for _, raw := range arr {
				ingestParsed(ctx, svc, raw, agent, sessionID)
			}
			return nil
		}
		ingestParsed(ctx, svc, data, agent, sessionID)
		return nil
	}

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ingestParsed(ctx, svc, []byte(line), agent, sessionID)
	}
	return sc.Err()
}

func ingestParsed(ctx context.Context, svc *ingest.Service, raw []byte, agent, fallbackSID string) {
	for _, ce := range agentparse.ExtractCommands(raw, agent) {
		sid := ce.SessionID
		if sid == "" {
			sid = fallbackSID
		}
		ev := map[string]any{
			"schema":     "orma.event.v1",
			"ts":         ce.TS.UTC().Format(time.RFC3339Nano),
			"actor":      "agent",
			"agent":      ce.Agent,
			"kind":       "bash",
			"source":     ce.Agent,
			"command":    ce.Command,
			"session_id": sid,
			"tool":       ce.Tool,
			"cwd":        ce.CWD,
		}
		if ce.ParentID != "" {
			ev["parent_session_id"] = ce.ParentID
		}
		if ce.Goal != "" {
			ev["goal"] = ce.Goal
		}
		if ce.ExitCode != nil {
			ev["exit_code"] = *ce.ExitCode
		} else {
			ev["outcome"] = "unknown"
		}
		b, _ := json.Marshal(ev)
		_, _ = svc.IngestOne(ctx, b)
	}
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
