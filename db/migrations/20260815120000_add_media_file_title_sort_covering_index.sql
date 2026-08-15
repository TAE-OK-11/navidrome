-- +goose Up
-- +goose StatementBegin

-- Covering index for title-sorted, library-scoped song listing:
--   WHERE missing = ? AND library_id = ? ORDER BY order_title LIMIT n OFFSET m
-- Native and Subsonic song lists use this shape. Without it SQLite walks
-- media_file_order_title and fetches the table row for every skipped entry.
create index if not exists media_file_missing_library_order_title
on media_file(missing, library_id, order_title, id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop index if exists media_file_missing_library_order_title;
-- +goose StatementEnd
