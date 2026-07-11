package distill

import (
	"context"
	"database/sql"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/anandh8x/orma/internal/store"
	"github.com/anandh8x/orma/internal/workflow"
	"github.com/google/uuid"
)

// Service compresses sessions into workflows and fixes.
type Service struct {
	Store    *store.Store
	Workflow *workflow.Service
}

// DistillSession builds a workflow from a session's non-noise commands.
func (s *Service) DistillSession(ctx context.Context, sessionID, name string) (*workflow.Workflow, error) {
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT COALESCE(command,''), COALESCE(outcome,''), COALESCE(stderr_fingerprint,''),
		       COALESCE(project_root,''), COALESCE(actor,''), COALESCE(agent,'')
		FROM events
		WHERE session_id = ? AND is_noise = 0 AND command IS NOT NULL AND command != ''
		ORDER BY ts ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []workflow.Step
	var project, origin string
	var failFPs []string
	seen := map[string]int{}

	for rows.Next() {
		var cmd, outcome, fp, proj, actor, agent string
		if err := rows.Scan(&cmd, &outcome, &fp, &proj, &actor, &agent); err != nil {
			return nil, err
		}
		if project == "" {
			project = proj
		}
		if origin == "" {
			if actor == "agent" {
				origin = "agent"
			} else {
				origin = "human"
			}
		}
		// collapse exact consecutive duplicates, keep last outcome
		key := strings.TrimSpace(cmd)
		if n, ok := seen[key]; ok && n == len(steps)-1 && len(steps) > 0 && steps[len(steps)-1].Command == key {
			steps[len(steps)-1].Outcome = outcome
			continue
		}
		seen[key] = len(steps)
		steps = append(steps, workflow.Step{Command: cmd, Outcome: outcome})
		if outcome == "fail" && fp != "" {
			failFPs = append(failFPs, fp)
		}
		if outcome == "fail" && fp == "" {
			failFPs = append(failFPs, fingerprintCmd(cmd))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("session has no distillable commands")
	}

	// Prefer keep fail->success chains and verification-ish tail; drop pure success noise mid-explore if many steps
	steps = compressSteps(steps)

	if name == "" {
		name = "session-" + short(sessionID)
	}

	w, err := s.Workflow.Upsert(ctx, name, project, origin, sessionID, steps)
	if err != nil {
		return nil, err
	}

	// Link fixes for earlier fail fingerprints if session ended with a success step.
	if hasSuccess(steps) {
		for _, fp := range unique(failFPs) {
			_ = s.upsertFix(ctx, fp, w.ID, sessionID)
		}
	}

	// Mark session closed/success
	_, _ = s.Store.DB().ExecContext(ctx, `
		UPDATE sessions SET status = 'success', ended_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), sessionID,
	)

	return w, nil
}

// DistillLast distills the most recent session.
func (s *Service) DistillLast(ctx context.Context, name string) (*workflow.Workflow, error) {
	var id string
	err := s.Store.DB().QueryRowContext(ctx, `
		SELECT id FROM sessions ORDER BY last_event_at DESC LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no sessions")
	}
	if err != nil {
		return nil, err
	}
	return s.DistillSession(ctx, id, name)
}

// AutoDistillOpen closes idle-looking open agent sessions and distills them.
// Called by daemon periodically with a simple "not updated in 2 min" heuristic for ended work.
func (s *Service) AutoDistillStale(ctx context.Context, olderThan time.Duration) (int, error) {
	if olderThan <= 0 {
		olderThan = 2 * time.Minute
	}
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339Nano)
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT id FROM sessions
		WHERE status = 'open' AND last_event_at < ?
		ORDER BY last_event_at ASC
		LIMIT 20`, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	n := 0
	for _, id := range ids {
		if _, err := s.DistillSession(ctx, id, ""); err == nil {
			n++
		}
	}
	return n, nil
}

func (s *Service) upsertFix(ctx context.Context, fp, workflowID, sessionID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var id string
	err := s.Store.DB().QueryRowContext(ctx, `
		SELECT id FROM fixes WHERE error_fingerprint = ?`, fp).Scan(&id)
	if err == sql.ErrNoRows {
		id = uuid.NewString()
		_, err = s.Store.DB().ExecContext(ctx, `
			INSERT INTO fixes(id, error_fingerprint, resolution_workflow_id, session_id, examples_count, created_at, updated_at)
			VALUES (?,?,?,?,1,?,?)`,
			id, fp, workflowID, sessionID, now, now,
		)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.Store.DB().ExecContext(ctx, `
		UPDATE fixes SET resolution_workflow_id = ?, examples_count = examples_count + 1, updated_at = ?
		WHERE id = ?`, workflowID, now, id)
	return err
}

func compressSteps(steps []workflow.Step) []workflow.Step {
	if len(steps) <= 30 {
		return steps
	}
	// keep fails, last successes, and last 10
	var out []workflow.Step
	for i, st := range steps {
		if st.Outcome == "fail" || i >= len(steps)-10 {
			out = append(out, st)
			continue
		}
		// keep docker/git/test-ish
		c := strings.ToLower(st.Command)
		if strings.Contains(c, "docker") || strings.Contains(c, "kubectl") ||
			strings.Contains(c, "pytest") || strings.Contains(c, "npm") ||
			strings.Contains(c, "go test") || strings.Contains(c, "make ") {
			out = append(out, st)
		}
	}
	if len(out) == 0 {
		return steps
	}
	return out
}

func hasSuccess(steps []workflow.Step) bool {
	for _, s := range steps {
		if s.Outcome == "success" {
			return true
		}
	}
	return false
}

func fingerprintCmd(cmd string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(cmd)))
	return fmt.Sprintf("sha256:%x", sum[:8])
}

func unique(in []string) []string {
	m := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := m[s]; ok {
			continue
		}
		m[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
