-- +goose Up
update media_file
set
    bpm = coalesce(bpm, 0),
    duration = coalesce(duration, 0),
    bit_rate = coalesce(bit_rate, 0),
    sample_rate = coalesce(sample_rate, 0),
    channels = coalesce(channels, 0),
    disc_number = coalesce(disc_number, 0),
    track_number = coalesce(track_number, 0),
    year = coalesce(year, 0),
    size = coalesce(size, 0)
where
    bpm is null
    or duration is null
    or bit_rate is null
    or sample_rate is null
    or channels is null
    or disc_number is null
    or track_number is null
    or year is null
    or size is null;

-- +goose Down
select 1;
