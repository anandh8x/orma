package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the local SQLite memory.
type Store struct {
	db          *sql.DB
	path        string
	busyTimeout time.Duration
}

// Open opens or creates the database, enables WAL, migrates.
func Open(path string, busyTimeoutMS int) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Allow a few concurrent queries (scan + enrich). Writes still serialize via WAL.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

	s := &Store{
		db:          db,
		path:        path,
		busyTimeout: time.Duration(busyTimeoutMS) * time.Millisecond,
	}
	if s.busyTimeout <= 0 {
		s.busyTimeout = 5 * time.Second
	}

	if err := s.pragma(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) pragma() error {
	stmts := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		fmt.Sprintf("PRAGMA busy_timeout = %d", int(s.busyTimeout.Milliseconds())),
		"PRAGMA synchronous = NORMAL",
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	return nil
}

func (s *Store) migrate() error {
	ver := s.readSchemaVersion()

	if ver >= schemaVersion {
		return nil
	}

	if ver > 0 {
		if err := s.backup(); err != nil {
			return fmt.Errorf("backup before migrate: %w", err)
		}
	}

	for v := ver + 1; v <= schemaVersion; v++ {
		sqlText, ok := migrations[v]
		if !ok {
			return fmt.Errorf("missing migration for version %d", v)
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(sqlText); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate v%d: %w", v, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO meta(key, value) VALUES('schema_version', ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			fmt.Sprintf("%d", v),
		); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	// Ensure body column exists on older workflow rows from partial installs.
	_, _ = s.db.Exec(`ALTER TABLE workflows ADD COLUMN body TEXT`) // ignore if exists
	return nil
}

func (s *Store) readSchemaVersion() int {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&name)
	if err != nil {
		return 0
	}
	var v string
	err = s.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v)
	if err != nil {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(v, "%d", &n)
	return n
}

func (s *Store) backup() error {
	if s.path == "" {
		return nil
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	dst := s.path + ".bak-" + stamp
	in, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o600)
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB exposes the underlying handle.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Path returns the database file path.
func (s *Store) Path() string {
	return s.path
}

// WithRetry runs fn, retrying on busy/locked errors.
func (s *Store) WithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	deadline := time.Now().Add(s.busyTimeout)
	var last error
	for {
		last = fn(ctx)
		if last == nil {
			return nil
		}
		if !isBusy(last) {
			return last
		}
		if time.Now().After(deadline) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "busy") || strings.Contains(msg, "locked")
}

// Ping checks the connection.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// SchemaVersion returns the stored schema version.
func (s *Store) SchemaVersion() (int, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v)
	if err != nil {
		return 0, err
	}
	var n int
	_, err = fmt.Sscanf(v, "%d", &n)
	return n, err
}

// UpsertFTS indexes a memory document.
func (s *Store) UpsertFTS(ctx context.Context, refType, refID, projectRoot, title, body string) error {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM memory_fts WHERE ref_type = ? AND ref_id = ?`, refType, refID)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memory_fts(body, ref_type, ref_id, project_root, title) VALUES (?,?,?,?,?)`,
		body, refType, refID, projectRoot, title,
	)
	return err
}

// EnqueueEmbed queues a ref for background embedding.
func (s *Store) EnqueueEmbed(ctx context.Context, refType, refID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO embed_queue(id, ref_type, ref_id, created_at)
		VALUES (?,?,?,?)
		ON CONFLICT(ref_type, ref_id) DO NOTHING`,
		refType+":"+refID, refType, refID, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}
