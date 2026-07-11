package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/anandh8x/orma/internal/store"
	"github.com/google/uuid"
)

// Step is one command in a workflow.
type Step struct {
	Command string
	Outcome string
}

// Workflow is a reusable command sequence.
type Workflow struct {
	ID           string
	Name         string
	ProjectRoot  string
	Origin       string
	SessionID    string
	Body         string
	Pinned       bool
	SuccessCount int
	UseCount     int
	Steps        []Step
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Service manages workflows, notes, pins.
type Service struct {
	Store *store.Store
}

// SaveNamed creates or replaces a named workflow for a project from recent non-noise events.
func (s *Service) SaveNamed(ctx context.Context, name, projectRoot string, limit int) (*Workflow, error) {
	if limit <= 0 {
		limit = 20
	}
	steps, err := s.recentSteps(ctx, projectRoot, limit)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 && projectRoot != "" {
		// fall back to any recent commands if this project is empty
		steps, err = s.recentSteps(ctx, "", limit)
		if err != nil {
			return nil, err
		}
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("no commands to save")
	}

	return s.Upsert(ctx, name, projectRoot, "human", "", steps)
}

// Upsert replaces workflow by name+project when name is set.
func (s *Service) Upsert(ctx context.Context, name, projectRoot, origin, sessionID string, steps []Step) (*Workflow, error) {
	now := time.Now().UTC()
	body := bodyFromSteps(steps)

	var existingID string
	if name != "" {
		_ = s.Store.DB().QueryRowContext(ctx, `
			SELECT id FROM workflows WHERE name = ? AND COALESCE(project_root,'') = ?`,
			name, projectRoot,
		).Scan(&existingID)
	}

	id := existingID
	if id == "" {
		id = uuid.NewString()
	}

	err := s.Store.WithRetry(ctx, func(ctx context.Context) error {
		tx, err := s.Store.DB().BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		if existingID != "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_steps WHERE workflow_id = ?`, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE workflows SET body=?, origin=?, session_id=?, updated_at=?,
					success_count = success_count
				WHERE id = ?`,
				body, origin, nullStr(sessionID), now.Format(time.RFC3339Nano), id,
			); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO workflows(id, name, project_root, origin, session_id, body, pinned, success_count, use_count, created_at, updated_at)
				VALUES (?,?,?,?,?,?,0,0,0,?,?)`,
				id, nullStr(name), nullStr(projectRoot), origin, nullStr(sessionID), body,
				now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
			); err != nil {
				return err
			}
		}

		for i, st := range steps {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO workflow_steps(workflow_id, idx, command, outcome) VALUES (?,?,?,?)`,
				id, i, st.Command, nullStr(st.Outcome),
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return nil, err
	}

	_ = s.Store.UpsertFTS(ctx, "workflow", id, projectRoot, name, body)
	_ = s.Store.EnqueueEmbed(ctx, "workflow", id)

	return s.Get(ctx, id)
}

// Get loads a workflow with steps.
func (s *Service) Get(ctx context.Context, id string) (*Workflow, error) {
	w := &Workflow{}
	var name, project, origin, session, body, created, updated sql.NullString
	var pinned, success, use int
	err := s.Store.DB().QueryRowContext(ctx, `
		SELECT id, name, project_root, origin, session_id, body, pinned, success_count, use_count, created_at, updated_at
		FROM workflows WHERE id = ?`, id,
	).Scan(&w.ID, &name, &project, &origin, &session, &body, &pinned, &success, &use, &created, &updated)
	if err != nil {
		return nil, err
	}
	w.Name = name.String
	w.ProjectRoot = project.String
	w.Origin = origin.String
	w.SessionID = session.String
	w.Body = body.String
	w.Pinned = pinned == 1
	w.SuccessCount = success
	w.UseCount = use

	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT command, COALESCE(outcome,'') FROM workflow_steps WHERE workflow_id = ? ORDER BY idx`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.Command, &st.Outcome); err != nil {
			return nil, err
		}
		w.Steps = append(w.Steps, st)
	}
	return w, rows.Err()
}

// ListByProject returns workflows for a project path.
func (s *Service) ListByProject(ctx context.Context, projectRoot string, limit int) ([]Workflow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT id FROM workflows
		WHERE ? = '' OR project_root = ?
		ORDER BY pinned DESC, updated_at DESC
		LIMIT ?`, projectRoot, projectRoot, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workflow
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		w, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// Delete removes a workflow.
func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.Store.DB().ExecContext(ctx, `DELETE FROM workflows WHERE id = ?`, id)
	if err != nil {
		return err
	}
	_, _ = s.Store.DB().ExecContext(ctx, `DELETE FROM memory_fts WHERE ref_type = 'workflow' AND ref_id = ?`, id)
	return nil
}

// AddNote stores a note and indexes it.
func (s *Service) AddNote(ctx context.Context, text, projectRoot string) (string, error) {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.Store.DB().ExecContext(ctx, `
		INSERT INTO notes(id, text, project_root, created_at) VALUES (?,?,?,?)`,
		id, text, nullStr(projectRoot), now,
	)
	if err != nil {
		return "", err
	}
	_ = s.Store.UpsertFTS(ctx, "note", id, projectRoot, "note", text)
	_ = s.Store.EnqueueEmbed(ctx, "note", id)
	return id, nil
}

// Pin pins a ref.
func (s *Service) Pin(ctx context.Context, refType, refID string) error {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.Store.DB().ExecContext(ctx, `
		INSERT INTO pins(id, ref_type, ref_id, created_at) VALUES (?,?,?,?)
		ON CONFLICT(ref_type, ref_id) DO NOTHING`,
		id, refType, refID, now,
	)
	if err != nil {
		return err
	}
	if refType == "workflow" {
		_, _ = s.Store.DB().ExecContext(ctx, `UPDATE workflows SET pinned = 1 WHERE id = ?`, refID)
	}
	return nil
}

// Unpin removes a pin.
func (s *Service) Unpin(ctx context.Context, refType, refID string) error {
	_, err := s.Store.DB().ExecContext(ctx, `DELETE FROM pins WHERE ref_type = ? AND ref_id = ?`, refType, refID)
	if refType == "workflow" {
		_, _ = s.Store.DB().ExecContext(ctx, `UPDATE workflows SET pinned = 0 WHERE id = ?`, refID)
	}
	return err
}

func (s *Service) recentSteps(ctx context.Context, projectRoot string, limit int) ([]Step, error) {
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT COALESCE(command,''), COALESCE(outcome,'')
		FROM events
		WHERE is_noise = 0 AND command IS NOT NULL AND command != ''
		  AND (? = '' OR project_root = ?)
		ORDER BY ts DESC
		LIMIT ?`, projectRoot, projectRoot, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []Step
	for rows.Next() {
		var cmd, outcome string
		if err := rows.Scan(&cmd, &outcome); err != nil {
			return nil, err
		}
		steps = append(steps, Step{Command: cmd, Outcome: outcome})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	return steps, nil
}

func bodyFromSteps(steps []Step) string {
	var b strings.Builder
	for _, s := range steps {
		b.WriteString(s.Command)
		b.WriteByte('\n')
	}
	return b.String()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
