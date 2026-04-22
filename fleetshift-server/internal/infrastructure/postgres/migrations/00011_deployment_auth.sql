-- +goose Up
ALTER TABLE deployments ADD COLUMN auth JSONB NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE deployments DROP COLUMN auth;
