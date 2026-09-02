-- +goose Up
CREATE TABLE IF NOT EXISTS user_api_key (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    lookup_prefix TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    last_used_at DATETIME,
    expires_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY(user_id) REFERENCES user(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_api_key_lookup_prefix ON user_api_key(lookup_prefix);
CREATE INDEX IF NOT EXISTS idx_user_api_key_user_id ON user_api_key(user_id);

-- +goose Down
DROP TABLE IF EXISTS user_api_key;
