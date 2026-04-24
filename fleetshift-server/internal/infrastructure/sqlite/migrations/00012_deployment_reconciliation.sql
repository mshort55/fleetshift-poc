-- +goose Up
ALTER TABLE deployments ADD COLUMN generation INTEGER NOT NULL DEFAULT 1;
ALTER TABLE deployments ADD COLUMN observed_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN reconciling INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE deployments DROP COLUMN generation;
ALTER TABLE deployments DROP COLUMN observed_generation;
ALTER TABLE deployments DROP COLUMN reconciling;
