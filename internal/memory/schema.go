package memory

// CurrentSchemaVersion is the latest supported Memory schema.
const CurrentSchemaVersion = 1

// migrationV1 creates observations, sessions, and FTS. schema_migrations is
// owned by the migrator (created before applying ordered migrations).
const migrationV1 = `
CREATE TABLE memory_sessions (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  started_at TEXT NOT NULL,
  ended_at TEXT,
  summary TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX memory_sessions_project_idx ON memory_sessions(project_id);

CREATE TABLE memory_observations (
  id TEXT PRIMARY KEY NOT NULL,
  project_id TEXT NOT NULL,
  session_id TEXT,
  type TEXT NOT NULL,
  scope TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  topic_key TEXT,
  source_kind TEXT,
  source_ref TEXT,
  content_hash TEXT NOT NULL,
  duplicate_count INTEGER NOT NULL DEFAULT 1,
  revision_count INTEGER NOT NULL DEFAULT 0,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (session_id) REFERENCES memory_sessions(id)
);

CREATE UNIQUE INDEX memory_observations_hash_idx
  ON memory_observations(project_id, content_hash);

CREATE UNIQUE INDEX memory_observations_topic_idx
  ON memory_observations(project_id, scope, topic_key)
  WHERE topic_key IS NOT NULL AND topic_key != '';

CREATE INDEX memory_observations_project_idx
  ON memory_observations(project_id);

CREATE VIRTUAL TABLE memory_observations_fts USING fts5(
  title,
  content,
  topic_key,
  id UNINDEXED
);
`
