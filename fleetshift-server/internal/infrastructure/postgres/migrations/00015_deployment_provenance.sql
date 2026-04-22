-- +goose Up
ALTER TABLE deployments ADD COLUMN provenance JSONB;

-- +goose Down
ALTER TABLE deployments DROP COLUMN provenance;
