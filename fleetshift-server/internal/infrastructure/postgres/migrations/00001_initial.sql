-- +goose Up
CREATE TABLE targets (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    labels     JSONB NOT NULL DEFAULT '{}',
    properties JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE deployments (
    id                 TEXT PRIMARY KEY,
    manifest_strategy  JSONB NOT NULL,
    placement_strategy JSONB NOT NULL,
    rollout_strategy   JSONB,
    resolved_targets   JSONB NOT NULL DEFAULT '[]',
    state              TEXT NOT NULL DEFAULT 'pending'
);

CREATE TABLE delivery_records (
    deployment_id TEXT NOT NULL,
    target_id     TEXT NOT NULL,
    manifests     JSONB NOT NULL DEFAULT '[]',
    state         TEXT NOT NULL DEFAULT 'pending',
    updated_at    TEXT NOT NULL DEFAULT NOW(),
    PRIMARY KEY (deployment_id, target_id)
);

-- +goose Down
DROP TABLE delivery_records;
DROP TABLE deployments;
DROP TABLE targets;
