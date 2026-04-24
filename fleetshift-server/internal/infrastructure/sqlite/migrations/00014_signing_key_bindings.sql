-- +goose Up
CREATE TABLE signing_key_bindings (
    id TEXT PRIMARY KEY,
    subject_id TEXT NOT NULL,
    issuer TEXT NOT NULL,
    public_key_jwk BLOB NOT NULL,
    algorithm TEXT NOT NULL,
    key_binding_doc BLOB NOT NULL,
    key_binding_signature BLOB NOT NULL,
    identity_token TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE INDEX idx_skb_subject ON signing_key_bindings(subject_id, issuer);

-- +goose Down
DROP INDEX idx_skb_subject;
DROP TABLE signing_key_bindings;
