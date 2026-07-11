package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/event"
	"github.com/anandh8x/orma/internal/noise"
	"github.com/anandh8x/orma/internal/project"
	"github.com/anandh8x/orma/internal/session"
	"github.com/anandh8x/orma/internal/store"
	"github.com/google/uuid"
)

// Service writes validated events into the store.
type Service struct {
	Store  *store.Store
	Config *config.Config
}

// Result is returned after a successful ingest.
type Result struct {
	EventID   string `json:"event_id"`
	SessionID string `json:"session_id"`
	Noise     bool   `json:"noise"`
	Skipped   bool   `json:"skipped,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// IngestOne validates, normalizes, and persists a single event.
func (s *Service) IngestOne(ctx context.Context, raw []byte) (*Result, error) {
	ev, err := event.ParseJSON(raw)
	if err != nil {
		return nil, err
	}
	ev.Normalize()
	if err := ev.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	if noise.CWDIgnored(ev.CWD, s.Config.IgnoreCWD) {
		return &Result{Skipped: true, Reason: "cwd ignored"}, nil
	}

	if ev.ProjectRoot == "" && ev.CWD != "" {
		ev.ProjectRoot = project.Resolve(ev.CWD)
	}

	if s.Config.Redact {
		applyRedact(ev)
	}

	ev.StderrExcerpt = event.TruncateExcerpt(ev.StderrExcerpt, s.Config.StderrExcerptMax)

	cmd := ""
	if ev.Command != nil {
		cmd = *ev.Command
	}
	// Fail fingerprints help Error->Fix linking when stderr is missing.
	if ev.Outcome == "fail" && ev.StderrFingerprint == "" {
		if ev.StderrExcerpt != "" {
			ev.StderrFingerprint = fingerprint(ev.StderrExcerpt)
		} else if cmd != "" {
			ev.StderrFingerprint = fingerprint("cmd:" + cmd)
		}
	}
	isNoise := noise.IsNoiseCommand(cmd, s.Config.NoiseCommands)

	if ev.EventID == "" {
		ev.EventID = uuid.NewString()
	}

	var sessionID string
	err = s.Store.WithRetry(ctx, func(ctx context.Context) error {
		res := s.Store.DB()
		sid, err := s.attachSession(ctx, res, ev)
		if err != nil {
			return err
		}
		sessionID = sid
		return insertEvent(ctx, res, ev, sid, isNoise)
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		EventID:   ev.EventID,
		SessionID: sessionID,
		Noise:     isNoise,
	}, nil
}

func (s *Service) attachSession(ctx context.Context, db *sql.DB, ev *event.Event) (string, error) {
	r := &session.Resolver{DB: db}
	return r.Attach(ctx, session.AttachInput{
		TS:                ev.TS,
		Actor:             ev.Actor,
		Agent:             ev.Agent,
		Source:            ev.Source,
		ProjectRoot:       ev.ProjectRoot,
		ProducerSessionID: ev.SessionID,
		ParentSessionID:   ev.ParentSessionID,
		Goal:              ev.Goal,
		Idle:              s.Config.SessionIdle.Duration,
	})
}

func insertEvent(ctx context.Context, db *sql.DB, ev *event.Event, sessionID string, isNoise bool) error {
	tagsJSON, _ := json.Marshal(ev.Tags)
	metaJSON, _ := json.Marshal(ev.Meta)
	if ev.Tags == nil {
		tagsJSON = nil
	}
	if ev.Meta == nil {
		metaJSON = nil
	}

	var exit any
	if ev.ExitCode != nil {
		exit = *ev.ExitCode
	}
	var dur any
	if ev.DurationMS != nil {
		dur = *ev.DurationMS
	}
	var cmd any
	if ev.Command != nil {
		cmd = *ev.Command
	}

	noiseInt := 0
	if isNoise {
		noiseInt = 1
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO events(
			id, schema_id, ts, actor, kind, source, agent, session_id, parent_session_id,
			command, text, cwd, project_root, exit_code, duration_ms, outcome,
			stderr_excerpt, stderr_fingerprint, stdout_fingerprint, tool, goal,
			host, shell, tags_json, raw_ref, meta_json, is_noise, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ev.EventID,
		ev.Schema,
		ev.TS.UTC().Format(time.RFC3339Nano),
		ev.Actor,
		ev.Kind,
		ev.Source,
		nullStr(ev.Agent),
		sessionID,
		nullStr(ev.ParentSessionID),
		cmd,
		nullStr(ev.Text),
		nullStr(ev.CWD),
		nullStr(ev.ProjectRoot),
		exit,
		dur,
		nullStr(ev.Outcome),
		nullStr(ev.StderrExcerpt),
		nullStr(ev.StderrFingerprint),
		nullStr(ev.StdoutFingerprint),
		nullStr(ev.Tool),
		nullStr(ev.Goal),
		nullStr(ev.Host),
		nullStr(ev.Shell),
		nullableJSON(tagsJSON),
		nullStr(ev.RawRef),
		nullableJSON(metaJSON),
		noiseInt,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			// idempotent re-ingest of same event id
			return nil
		}
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableJSON(b []byte) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}

// Optional redaction (off by default). Very small starter set.
func applyRedact(ev *event.Event) {
	if ev.Command != nil {
		s := redactString(*ev.Command)
		ev.Command = &s
	}
	ev.Goal = redactString(ev.Goal)
	ev.Text = redactString(ev.Text)
	ev.StderrExcerpt = redactString(ev.StderrExcerpt)
}

func redactString(s string) string {
	// Keep this conservative; full patterns can grow later.
	repl := []struct{ old, new string }{
		// bearer tokens
	}
	_ = repl
	// simple patterns via strings
	out := s
	out = redactKeyValue(out, "api_key")
	out = redactKeyValue(out, "API_KEY")
	out = redactKeyValue(out, "password")
	out = redactKeyValue(out, "PASSWORD")
	out = redactKeyValue(out, "token")
	out = redactKeyValue(out, "TOKEN")
	if strings.Contains(strings.ToLower(out), "authorization: bearer ") {
		// crude
		idx := strings.Index(strings.ToLower(out), "authorization: bearer ")
		if idx >= 0 {
			end := idx + len("authorization: bearer ")
			out = out[:end] + "${REDACTED:bearer}"
		}
	}
	return out
}

func fingerprint(s string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return fmt.Sprintf("sha256:%x", sum[:16])
}

func redactKeyValue(s, key string) string {
	// KEY=value or key=value
	upper := s
	for {
		i := strings.Index(upper, key+"=")
		if i < 0 {
			break
		}
		start := i + len(key) + 1
		end := start
		for end < len(s) && s[end] != ' ' && s[end] != '\'' && s[end] != '"' && s[end] != '\n' {
			end++
		}
		s = s[:start] + "${REDACTED:" + strings.ToLower(key) + "}" + s[end:]
		upper = s
		// prevent infinite loop
		if !strings.Contains(s[start:], key+"=") {
			break
		}
	}
	return s
}
