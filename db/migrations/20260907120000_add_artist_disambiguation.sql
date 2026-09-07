-- +goose Up
ALTER TABLE artist ADD COLUMN disambiguation TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE artist DROP COLUMN disambiguation;
