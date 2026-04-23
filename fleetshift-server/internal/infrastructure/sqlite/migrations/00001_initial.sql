-- +goose Up
CREATE TABLE targets (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL UNIQUE,
    type                    TEXT NOT NULL DEFAULT '',
    state                   TEXT NOT NULL DEFAULT 'ready',
    labels                  TEXT NOT NULL DEFAULT '{}',
    properties              TEXT NOT NULL DEFAULT '{}',
    accepted_resource_types TEXT NOT NULL DEFAULT '[]',
    inventory_item_id       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE deployments (
    id                  TEXT PRIMARY KEY,
    uid                 TEXT NOT NULL DEFAULT '',
    manifest_strategy   TEXT NOT NULL,
    placement_strategy  TEXT NOT NULL,
    rollout_strategy    TEXT,
    resolved_targets    TEXT NOT NULL DEFAULT '[]',
    state               TEXT NOT NULL DEFAULT 'creating',
    auth                TEXT NOT NULL DEFAULT '{}',
    provenance          TEXT,
    generation          INTEGER NOT NULL DEFAULT 1,
    observed_generation INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now')),
    etag                TEXT NOT NULL DEFAULT ''
);

CREATE TABLE delivery_records (
    deployment_id TEXT NOT NULL,
    target_id     TEXT NOT NULL,
    id            TEXT NOT NULL DEFAULT '',
    manifests     TEXT NOT NULL DEFAULT '[]',
    state         TEXT NOT NULL DEFAULT 'pending',
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (deployment_id, target_id)
);

CREATE TABLE auth_methods (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    config_json TEXT NOT NULL
);

CREATE TABLE vault_secrets (
    ref  TEXT PRIMARY KEY,
    val  BLOB NOT NULL
);

CREATE TABLE inventory_items (
    id                 TEXT PRIMARY KEY,
    type               TEXT NOT NULL,
    name               TEXT NOT NULL,
    properties         TEXT NOT NULL DEFAULT '{}',
    labels             TEXT NOT NULL DEFAULT '{}',
    source_delivery_id TEXT,
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_inventory_items_type ON inventory_items(type);

CREATE TABLE signer_enrollments (
    id               TEXT PRIMARY KEY,
    subject_id       TEXT NOT NULL,
    issuer           TEXT NOT NULL,
    identity_token   TEXT NOT NULL,
    registry_subject TEXT NOT NULL,
    registry_id      TEXT NOT NULL,
    created_at       TEXT NOT NULL,
    expires_at       TEXT NOT NULL
);

CREATE INDEX idx_se_subject ON signer_enrollments(subject_id, issuer);

-- +goose Down
DROP INDEX IF EXISTS idx_se_subject;
DROP TABLE IF EXISTS signer_enrollments;
DROP TABLE IF EXISTS inventory_items;
DROP TABLE IF EXISTS vault_secrets;
DROP TABLE IF EXISTS auth_methods;
DROP TABLE IF EXISTS delivery_records;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS targets;
