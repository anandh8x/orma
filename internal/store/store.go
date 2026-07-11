package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the local SQLite memory.
type Store struct {
	db           *sql.DB
	path         string
	busyTimeout  time.Duration
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
	// One writer-friendly setup; still allow concurrent readers.
	db.SetMaxOpenConns(1)

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
		"PRAGMA busy_timeout = " + fmt.Sprintf("%d", int(s.busyTimeout.Milliseconds())),
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
	var ver int
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&ver)
	if err != nil && err != sql.ErrNoRows {
		// table may not exist yet
		ver = 0
	}
	if err == sql.ErrNoRows {
		ver = 0
	}

	// Detect missing meta table
	var name string
	err = s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&name)
	if err == sql.ErrNoRows {
		ver = 0
	} else if err != nil {
		// try create from scratch
		ver = 0
	}

	if ver >= schemaVersion {
		return nil
	}

	// Backup existing db file before applying migrations when upgrading.
	if ver > 0 {
		if err := s.backup(); err != nil {
			return fmt.Errorf("backup before migrate: %w", err)
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Apply all migrations from current to latest (v1 is full schema for now).
	if ver < 1 {
		if _, err := tx.Exec(migrations[0]); err != nil {
			return fmt.Errorf("migrate v1: %w", err)
		}
	}

	if _, err := tx.Exec(
		`INSERT INTO meta(key, value) VALUES('schema_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		fmt.Sprintf("%d", schemaVersion),
	); err != nil {
		return err
	}

	return tx.Commit()
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

// DB exposes the underlying handle for packages in store (events, sessions).
func (s *Store) DB() *sql.DB {
	return s.db
}

// Path returns the database file path.
func (s *Store) Path() string {
	return s.path
}

// WithRetry runs fn, retrying on SQLITE_BUSY style errors.
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
	msg := err.Error()
	return contains(msg, "busy") || contains(msg, "locked") || contains(msg, "SQLITE_BUSY")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
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
