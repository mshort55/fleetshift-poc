-- +goose Up
CREATE TABLE IF NOT EXISTS vault_secrets (
    ref  TEXT PRIMARY KEY,
    val  BYTEA NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS vault_secrets;
