-- +goose Up
ALTER TABLE deployments ADD COLUMN auth TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE deployments DROP COLUMN auth;
