package runstate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/anandh8x/orma/internal/store"
	"github.com/google/uuid"
)

// Active run tracking for step-through use.
type Active struct {
	ID         string
	WorkflowID string
	StepIdx    int
	Status     string
	LastExit   *int
}

// Service manages run_state rows.
type Service struct {
	Store *store.Store
}

// Start begins a workflow run at step 0.
func (s *Service) Start(ctx context.Context, workflowID string) (*Active, error) {
	// clear other active
	_, _ = s.Store.DB().ExecContext(ctx, `UPDATE run_state SET status = 'aborted' WHERE status = 'active'`)
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.Store.DB().ExecContext(ctx, `
		INSERT INTO run_state(id, workflow_id, step_idx, status, updated_at)
		VALUES (?,?,0,'active',?)`, id, workflowID, now)
	if err != nil {
		return nil, err
	}
	return &Active{ID: id, WorkflowID: workflowID, StepIdx: 0, Status: "active"}, nil
}

// Current returns the active run if any.
func (s *Service) Current(ctx context.Context) (*Active, error) {
	a := &Active{}
	var last sql.NullInt64
	err := s.Store.DB().QueryRowContext(ctx, `
		SELECT id, workflow_id, step_idx, status, last_exit
		FROM run_state WHERE status = 'active'
		ORDER BY updated_at DESC LIMIT 1`,
	).Scan(&a.ID, &a.WorkflowID, &a.StepIdx, &a.Status, &last)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if last.Valid {
		v := int(last.Int64)
		a.LastExit = &v
	}
	return a, nil
}

// RecordExit updates after a command; advances on success unless stop requested.
func (s *Service) RecordExit(ctx context.Context, exitCode int) (*Active, error) {
	a, err := s.Current(ctx)
	if err != nil || a == nil {
		return a, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if exitCode != 0 {
		_, err = s.Store.DB().ExecContext(ctx, `
			UPDATE run_state SET last_exit = ?, status = 'failed', updated_at = ? WHERE id = ?`,
			exitCode, now, a.ID)
		a.Status = "failed"
		a.LastExit = &exitCode
		return a, err
	}
	// advance
	a.StepIdx++
	_, err = s.Store.DB().ExecContext(ctx, `
		UPDATE run_state SET step_idx = ?, last_exit = 0, updated_at = ? WHERE id = ?`,
		a.StepIdx, now, a.ID)
	a.LastExit = &exitCode
	return a, err
}

// Skip advances without caring about exit.
func (s *Service) Skip(ctx context.Context) (*Active, error) {
	a, err := s.Current(ctx)
	if err != nil || a == nil {
		return a, err
	}
	a.StepIdx++
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.Store.DB().ExecContext(ctx, `
		UPDATE run_state SET step_idx = ?, updated_at = ? WHERE id = ?`,
		a.StepIdx, now, a.ID)
	return a, err
}

// Abort ends the run.
func (s *Service) Abort(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.Store.DB().ExecContext(ctx, `
		UPDATE run_state SET status = 'aborted', updated_at = ? WHERE status = 'active'`, now)
	return err
}

// Complete marks done.
func (s *Service) Complete(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.Store.DB().ExecContext(ctx, `
		UPDATE run_state SET status = 'done', updated_at = ? WHERE status = 'active'`, now)
	return err
}

// Retry resets failed run to active without advancing.
func (s *Service) Retry(ctx context.Context) (*Active, error) {
	a, err := s.Current(ctx)
	if err != nil {
		return nil, err
	}
	if a != nil && a.Status == "active" {
		return a, nil
	}
	// last failed
	a = &Active{}
	var last sql.NullInt64
	err = s.Store.DB().QueryRowContext(ctx, `
		SELECT id, workflow_id, step_idx, status, last_exit FROM run_state
		WHERE status = 'failed' ORDER BY updated_at DESC LIMIT 1`,
	).Scan(&a.ID, &a.WorkflowID, &a.StepIdx, &a.Status, &last)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no failed run to retry")
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.Store.DB().ExecContext(ctx, `
		UPDATE run_state SET status = 'active', updated_at = ? WHERE id = ?`, now, a.ID)
	a.Status = "active"
	return a, err
}
