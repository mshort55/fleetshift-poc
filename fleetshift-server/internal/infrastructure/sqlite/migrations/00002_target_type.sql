-- +goose Up
ALTER TABLE targets ADD COLUMN type TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE targets DROP COLUMN type;
