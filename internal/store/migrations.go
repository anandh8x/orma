package store

const schemaVersion = 2

var migrations = map[int]string{
	1: migrationV1,
	2: migrationV2,
}

const migrationV1 = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id                  TEXT PRIMARY KEY,
  producer_session_id TEXT,
  parent_session_id   TEXT,
  actor_mix           TEXT NOT NULL DEFAULT 'human',
  agent               TEXT,
  source              TEXT,
  project_root        TEXT,
  goal                TEXT,
  started_at          TEXT NOT NULL,
  ended_at            TEXT,
  last_event_at       TEXT NOT NULL,
  status              TEXT NOT NULL DEFAULT 'open'
);

CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_root);
CREATE INDEX IF NOT EXISTS idx_sessions_last ON sessions(last_event_at);

CREATE TABLE IF NOT EXISTS events (
  id                  TEXT PRIMARY KEY,
  schema_id           TEXT NOT NULL,
  ts                  TEXT NOT NULL,
  actor               TEXT NOT NULL,
  kind                TEXT NOT NULL,
  source              TEXT NOT NULL,
  agent               TEXT,
  session_id          TEXT,
  parent_session_id   TEXT,
  command             TEXT,
  text                TEXT,
  cwd                 TEXT,
  project_root        TEXT,
  exit_code           INTEGER,
  duration_ms         INTEGER,
  outcome             TEXT,
  stderr_excerpt      TEXT,
  stderr_fingerprint  TEXT,
  stdout_fingerprint  TEXT,
  tool                TEXT,
  goal                TEXT,
  host                TEXT,
  shell               TEXT,
  tags_json           TEXT,
  raw_ref             TEXT,
  meta_json           TEXT,
  is_noise            INTEGER NOT NULL DEFAULT 0,
  created_at          TEXT NOT NULL,
  FOREIGN KEY(session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id);
CREATE INDEX IF NOT EXISTS idx_events_project ON events(project_root);
CREATE INDEX IF NOT EXISTS idx_events_outcome ON events(outcome);

CREATE TABLE IF NOT EXISTS workflows (
  id            TEXT PRIMARY KEY,
  name          TEXT,
  project_root  TEXT,
  origin        TEXT NOT NULL DEFAULT 'human',
  session_id    TEXT,
  body          TEXT,
  pinned        INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  use_count     INTEGER NOT NULL DEFAULT 0,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL,
  last_used_at  TEXT
);

CREATE TABLE IF NOT EXISTS workflow_steps (
  workflow_id TEXT NOT NULL,
  idx         INTEGER NOT NULL,
  command     TEXT NOT NULL,
  outcome     TEXT,
  PRIMARY KEY (workflow_id, idx),
  FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS notes (
  id           TEXT PRIMARY KEY,
  text         TEXT NOT NULL,
  project_root TEXT,
  session_id   TEXT,
  created_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pins (
  id         TEXT PRIMARY KEY,
  ref_type   TEXT NOT NULL,
  ref_id     TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(ref_type, ref_id)
);

CREATE TABLE IF NOT EXISTS fixes (
  id                     TEXT PRIMARY KEY,
  error_fingerprint      TEXT NOT NULL,
  resolution_workflow_id TEXT,
  session_id             TEXT,
  examples_count         INTEGER NOT NULL DEFAULT 1,
  created_at             TEXT NOT NULL,
  updated_at             TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_fixes_fp ON fixes(error_fingerprint);

CREATE TABLE IF NOT EXISTS embeddings (
  id         TEXT PRIMARY KEY,
  ref_type   TEXT NOT NULL,
  ref_id     TEXT NOT NULL,
  model      TEXT NOT NULL,
  dim        INTEGER NOT NULL,
  vector     BLOB NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(ref_type, ref_id, model)
);

CREATE TABLE IF NOT EXISTS run_state (
  id            TEXT PRIMARY KEY,
  workflow_id   TEXT NOT NULL,
  step_idx      INTEGER NOT NULL DEFAULT 0,
  status        TEXT NOT NULL DEFAULT 'active',
  last_exit     INTEGER,
  updated_at    TEXT NOT NULL
);
`

const migrationV2 = `
CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
  body,
  ref_type UNINDEXED,
  ref_id UNINDEXED,
  project_root UNINDEXED,
  title UNINDEXED,
  tokenize = 'porter unicode61'
);

CREATE TABLE IF NOT EXISTS embed_queue (
  id         TEXT PRIMARY KEY,
  ref_type   TEXT NOT NULL,
  ref_id     TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(ref_type, ref_id)
);
`
