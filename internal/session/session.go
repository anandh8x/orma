package session

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Info is a session row used while attaching events.
type Info struct {
	ID                string
	ProducerSessionID string
	ParentSessionID   string
	ActorMix          string
	Agent             string
	Source            string
	ProjectRoot       string
	Goal              string
	StartedAt         time.Time
	LastEventAt       time.Time
	Status            string
}

// AttachInput describes the incoming event for session resolution.
type AttachInput struct {
	TS                time.Time
	Actor             string
	Agent             string
	Source            string
	ProjectRoot       string
	ProducerSessionID string
	ParentSessionID   string
	Goal              string
	Idle              time.Duration
}

// Resolver finds or creates sessions.
type Resolver struct {
	DB *sql.DB
}

// Attach returns the internal session id for this event.
func (r *Resolver) Attach(ctx context.Context, in AttachInput) (string, error) {
	if in.Idle <= 0 {
		in.Idle = 20 * time.Minute
	}
	if in.TS.IsZero() {
		in.TS = time.Now().UTC()
	}

	// Explicit producer session id (agents): reuse same row if present.
	if in.ProducerSessionID != "" {
		var id string
		err := r.DB.QueryRowContext(ctx, `
			SELECT id FROM sessions
			WHERE producer_session_id = ? AND status = 'open'
			ORDER BY last_event_at DESC LIMIT 1`,
			in.ProducerSessionID,
		).Scan(&id)
		if err == nil {
			if err := r.touch(ctx, id, in); err != nil {
				return "", err
			}
			return id, nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
		return r.create(ctx, in)
	}

	// Human (or unscoped): open session for project+actor(+agent) within idle window.
	var (
		id          string
		lastEventAt string
		projectRoot string
		actorMix    string
		agent       string
	)
	err := r.DB.QueryRowContext(ctx, `
		SELECT id, last_event_at, COALESCE(project_root,''), actor_mix, COALESCE(agent,'')
		FROM sessions
		WHERE status = 'open'
		  AND actor_mix = ?
		  AND COALESCE(agent,'') = ?
		  AND COALESCE(source,'') = ?
		ORDER BY last_event_at DESC
		LIMIT 1`,
		actorMixFor(in.Actor), in.Agent, in.Source,
	).Scan(&id, &lastEventAt, &projectRoot, &actorMix, &agent)
	if err == sql.ErrNoRows {
		return r.create(ctx, in)
	}
	if err != nil {
		return "", err
	}

	last, err := time.Parse(time.RFC3339Nano, lastEventAt)
	if err != nil {
		last, err = time.Parse(time.RFC3339, lastEventAt)
		if err != nil {
			return r.create(ctx, in)
		}
	}

	// New session if idle gap or project root changed.
	if in.TS.Sub(last) > in.Idle || projectRoot != in.ProjectRoot {
		_ = r.close(ctx, id, last)
		return r.create(ctx, in)
	}

	if err := r.touch(ctx, id, in); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Resolver) create(ctx context.Context, in AttachInput) (string, error) {
	id := uuid.NewString()
	ts := in.TS.UTC().Format(time.RFC3339Nano)
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO sessions(
			id, producer_session_id, parent_session_id, actor_mix, agent, source,
			project_root, goal, started_at, last_event_at, status
		) VALUES (?,?,?,?,?,?,?,?,?,?, 'open')`,
		id,
		nullStr(in.ProducerSessionID),
		nullStr(in.ParentSessionID),
		actorMixFor(in.Actor),
		nullStr(in.Agent),
		nullStr(in.Source),
		nullStr(in.ProjectRoot),
		nullStr(in.Goal),
		ts,
		ts,
	)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return id, nil
}

func (r *Resolver) touch(ctx context.Context, id string, in AttachInput) error {
	ts := in.TS.UTC().Format(time.RFC3339Nano)
	_, err := r.DB.ExecContext(ctx, `
		UPDATE sessions SET last_event_at = ?,
			goal = CASE WHEN ? != '' THEN ? ELSE goal END
		WHERE id = ?`,
		ts, in.Goal, in.Goal, id,
	)
	return err
}

func (r *Resolver) close(ctx context.Context, id string, at time.Time) error {
	_, err := r.DB.ExecContext(ctx, `
		UPDATE sessions SET status = 'closed', ended_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

func actorMixFor(actor string) string {
	if actor == "agent" {
		return "agent"
	}
	return "human"
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
