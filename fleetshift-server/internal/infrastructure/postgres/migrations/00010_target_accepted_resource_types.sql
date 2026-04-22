-- +goose Up
ALTER TABLE targets ADD COLUMN accepted_resource_types JSONB NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE targets DROP COLUMN accepted_resource_types;
