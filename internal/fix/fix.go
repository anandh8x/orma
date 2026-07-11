package fix

import (
	"context"
	"fmt"
	"time"

	"github.com/anandh8x/orma/internal/store"
)

// Record is a fix row.
type Record struct {
	ID                   string
	ErrorFingerprint     string
	ResolutionWorkflowID string
	SessionID            string
	ExamplesCount        int
	UpdatedAt            string
}

// Service lists and indexes fixes.
type Service struct {
	Store *store.Store
}

// List returns recent fixes.
func (s *Service) List(ctx context.Context, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT id, error_fingerprint, COALESCE(resolution_workflow_id,''), COALESCE(session_id,''),
		       examples_count, updated_at
		FROM fixes ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.ErrorFingerprint, &r.ResolutionWorkflowID, &r.SessionID, &r.ExamplesCount, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns one fix.
func (s *Service) Get(ctx context.Context, id string) (*Record, error) {
	r := &Record{}
	err := s.Store.DB().QueryRowContext(ctx, `
		SELECT id, error_fingerprint, COALESCE(resolution_workflow_id,''), COALESCE(session_id,''),
		       examples_count, updated_at
		FROM fixes WHERE id = ?`, id,
	).Scan(&r.ID, &r.ErrorFingerprint, &r.ResolutionWorkflowID, &r.SessionID, &r.ExamplesCount, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// IndexFTS indexes a fix for search.
func (s *Service) IndexFTS(ctx context.Context, r Record) error {
	title := "fix"
	body := r.ErrorFingerprint + " " + r.ResolutionWorkflowID
	return s.Store.UpsertFTS(ctx, "fix", r.ID, "", title, body)
}

// ReindexAll rebuilds FTS for fixes.
func (s *Service) ReindexAll(ctx context.Context) (int, error) {
	list, err := s.List(ctx, 10000)
	if err != nil {
		return 0, err
	}
	for _, r := range list {
		if err := s.IndexFTS(ctx, r); err != nil {
			return 0, err
		}
	}
	return len(list), nil
}

// FindByFingerprint looks up a fix.
func (s *Service) FindByFingerprint(ctx context.Context, fp string) (*Record, error) {
	r := &Record{}
	err := s.Store.DB().QueryRowContext(ctx, `
		SELECT id, error_fingerprint, COALESCE(resolution_workflow_id,''), COALESCE(session_id,''),
		       examples_count, updated_at
		FROM fixes WHERE error_fingerprint = ?`, fp,
	).Scan(&r.ID, &r.ErrorFingerprint, &r.ResolutionWorkflowID, &r.SessionID, &r.ExamplesCount, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// FormatHuman short display.
func FormatHuman(r Record) string {
	return fmt.Sprintf("%s  examples=%d  wf=%s  %s", short(r.ErrorFingerprint), r.ExamplesCount, short(r.ResolutionWorkflowID), r.UpdatedAt)
}

func short(s string) string {
	if len(s) > 24 {
		return s[:24]
	}
	return s
}

// TouchUpdated is a helper timestamp.
func TouchUpdated() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
