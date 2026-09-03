package persistence

import "database/sql"

// PluginRuntimeSchema is the namespaced plugin kv/task DDL. Production applies
// this via goose; tests that do not run full migrations can apply it directly.
const PluginRuntimeSchema = `
CREATE TABLE IF NOT EXISTS plugin_kvstore (
    plugin_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value BLOB NOT NULL,
    size INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME DEFAULT NULL,
    PRIMARY KEY (plugin_id, key)
);
CREATE INDEX IF NOT EXISTS idx_plugin_kvstore_expires_at ON plugin_kvstore(plugin_id, expires_at);

CREATE TABLE IF NOT EXISTS plugin_queue (
    plugin_id TEXT NOT NULL,
    name TEXT NOT NULL,
    concurrency INTEGER NOT NULL DEFAULT 1,
    max_retries INTEGER NOT NULL DEFAULT 0,
    backoff_ms INTEGER NOT NULL DEFAULT 1000,
    delay_ms INTEGER NOT NULL DEFAULT 0,
    retention_ms INTEGER NOT NULL DEFAULT 3600000,
    PRIMARY KEY (plugin_id, name)
);

CREATE TABLE IF NOT EXISTS plugin_task (
    id TEXT PRIMARY KEY,
    plugin_id TEXT NOT NULL,
    queue_name TEXT NOT NULL,
    payload BLOB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL,
    next_run_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (plugin_id, queue_name) REFERENCES plugin_queue(plugin_id, name)
);
CREATE INDEX IF NOT EXISTS idx_plugin_task_dequeue ON plugin_task(plugin_id, queue_name, status, next_run_at);
`

func EnsurePluginRuntimeSchema(db *sql.DB) error {
	_, err := db.Exec(PluginRuntimeSchema)
	return err
}
