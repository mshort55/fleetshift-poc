-- +goose Up
CREATE TABLE auth_methods (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    config_json TEXT NOT NULL
);

-- +goose Down
DROP TABLE auth_methods;
