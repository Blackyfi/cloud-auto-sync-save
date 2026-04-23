-- v1 schema. Phase 0 only uses `users`; the rest are pre-created so later
-- phases can populate them without further migrations.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE devices (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, name)
);

CREATE TABLE device_tokens (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    revoked_at TEXT
);

CREATE TABLE policies (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id   INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    config_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE snapshots_index (
    id            TEXT PRIMARY KEY,
    device_id     INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    started_at    TEXT NOT NULL,
    completed_at  TEXT,
    bytes_logical INTEGER NOT NULL DEFAULT 0,
    bytes_new     INTEGER NOT NULL DEFAULT 0,
    file_count    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_snapshots_device_started ON snapshots_index(device_id, started_at DESC);

CREATE TABLE replication_targets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    label      TEXT NOT NULL UNIQUE,
    mount_path TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE replication_runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id    INTEGER NOT NULL REFERENCES replication_targets(id) ON DELETE CASCADE,
    started_at   TEXT NOT NULL,
    completed_at TEXT,
    status       TEXT NOT NULL,
    bytes_copied INTEGER NOT NULL DEFAULT 0,
    error        TEXT
);

CREATE TABLE restore_tests (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id  TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    completed_at TEXT,
    status       TEXT NOT NULL,
    error        TEXT
);

-- smtp_password is stored plaintext in Phase 0 schema. Phase 4 will encrypt
-- it at rest with a server-side data key (see docs/UPGRADE.md not required —
-- this is a Phase 4 task already scoped in SPEC.md).
CREATE TABLE email_settings (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    smtp_host     TEXT NOT NULL,
    smtp_port     INTEGER NOT NULL,
    smtp_username TEXT NOT NULL,
    smtp_password TEXT NOT NULL,
    from_address  TEXT NOT NULL,
    recipients    TEXT NOT NULL,
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE email_outbox (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    subject    TEXT NOT NULL,
    body       TEXT NOT NULL,
    queued_at  TEXT NOT NULL DEFAULT (datetime('now')),
    sent_at    TEXT,
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT
);

CREATE TABLE audit_log (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    at     TEXT NOT NULL DEFAULT (datetime('now')),
    actor  TEXT,
    action TEXT NOT NULL,
    detail TEXT
);
