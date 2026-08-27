package metadata

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

func (md Metadata) ToMediaFile(libID int, folderID string) model.MediaFile {
	if md.mediaFileJSON == "" {
		log.Warn("Missing media_file_json from Rust worker", "file", md.filePath)
		return md.fallbackMediaFile(libID, folderID)
	}
	mf, ok := md.mediaFileFromRust(libID, folderID)
	if !ok {
		return md.fallbackMediaFile(libID, folderID)
	}
	return mf
}

func (md Metadata) fallbackMediaFile(libID int, folderID string) model.MediaFile {
	mf := model.MediaFile{
		LibraryID: libID,
		FolderID:  folderID,
		Path:      md.FilePath(),
		Suffix:    md.Suffix(),
		Size:      md.Size(),
		BirthTime: md.BirthTime(),
		UpdatedAt: md.ModTime(),
		Tags:      maps.Clone(md.tags),
	}
	mf.HasCoverArt = md.HasPicture()
	mf.Duration = md.Length()
	mf.BitRate = md.AudioProperties().BitRate
	mf.SampleRate = md.AudioProperties().SampleRate
	if bd := md.AudioProperties().BitDepth; bd > 0 {
		mf.BitDepth = new(bd)
	}
	mf.Channels = md.AudioProperties().Channels
	mf.Codec = md.AudioProperties().Codec
	mf.Lyrics = md.mapLyrics()
	return mf
}

func (md Metadata) mediaFileFromRust(libID int, folderID string) (model.MediaFile, bool) {
	var payload struct {
		model.MediaFile
		Participants map[string][]model.Participant `json:"participants"`
	}
	if err := json.Unmarshal([]byte(md.mediaFileJSON), &payload); err != nil {
		log.Warn("Rust media_file_json decode failed", "file", md.filePath, err)
		return model.MediaFile{}, false
	}
	mf := payload.MediaFile
	if len(payload.Participants) > 0 {
		mf.Participants = make(model.Participants, len(payload.Participants))
		for roleKey, list := range payload.Participants {
			role := model.RoleFromString(roleKey)
			if role == model.RoleInvalid {
				continue
			}
			for i := range list {
				if list[i].ID == "" {
					list[i].ID = md.artistID(list[i].Name)
				}
			}
			mf.Participants[role] = list
		}
	}
	mf.LibraryID = libID
	mf.FolderID = folderID
	mf.Path = md.FilePath()
	mf.Suffix = md.Suffix()
	mf.Size = md.Size()
	mf.BirthTime = md.BirthTime()
	mf.UpdatedAt = md.ModTime()
	mf.HasCoverArt = md.HasPicture()
	mf.Duration = md.Length()
	mf.BitRate = md.AudioProperties().BitRate
	mf.SampleRate = md.AudioProperties().SampleRate
	if bd := md.AudioProperties().BitDepth; bd > 0 {
		mf.BitDepth = new(bd)
	}
	mf.Channels = md.AudioProperties().Channels
	mf.Codec = md.AudioProperties().Codec
	if mf.Lyrics == "" {
		mf.Lyrics = md.mapLyrics()
	}
	mf.PID = md.trackPID(mf)
	mf.AlbumID = md.albumID(mf, conf.Server.PID.Album)
	mf.ArtistID = mf.Participants.First(model.RoleArtist).ID
	mf.AlbumArtistID = mf.Participants.First(model.RoleAlbumArtist).ID
	mf.OrderArtistName = mf.Participants.First(model.RoleArtist).OrderArtistName
	mf.OrderAlbumArtistName = mf.Participants.First(model.RoleAlbumArtist).OrderArtistName
	mf.SortArtistName = mf.Participants.First(model.RoleArtist).SortArtistName
	mf.SortAlbumArtistName = mf.Participants.First(model.RoleAlbumArtist).SortArtistName
	mf.Tags = maps.Clone(md.tags)
	for tag, conf := range model.TagMainMappings() {
		if !conf.Album {
			delete(mf.Tags, tag)
		}
	}
	return mf, true
}

func (md Metadata) AlbumID(mf model.MediaFile, pidConf string) string {
	return md.albumID(mf, pidConf)
}

func (md Metadata) mapLyrics() string {
	if md.lyricsJSON != "" {
		return md.lyricsJSON
	}

	rawLyrics := md.Pairs(model.TagLyrics)

	lyricList := make(model.LyricList, 0, len(rawLyrics))

	ctx := log.NewContext(context.Background(), "file", md.filePath)
	for _, raw := range rawLyrics {
		lang := raw.Key()
		text := raw.Value()

		lyrics, err := model.ParseLyrics(ctx, "", lang, []byte(text))
		if err != nil {
			log.Warn(ctx, "Unexpected failure occurred when parsing lyrics", err)
			continue
		}
		for _, lyric := range lyrics {
			if !lyric.IsEmpty() {
				lyricList = append(lyricList, lyric)
			}
		}
	}

	res, err := json.Marshal(lyricList)
	if err != nil {
		log.Warn("Unexpected error occurred when serializing lyrics", "file", md.filePath, err)
		return ""
	}
	return string(res)
}

func (md Metadata) mapDates() (date Date, originalDate Date, releaseDate Date) {
	// Start with defaults
	date = md.Date(model.TagRecordingDate)
	originalDate = md.Date(model.TagOriginalDate)
	releaseDate = md.Date(model.TagReleaseDate)

	// For some historic reason, taggers have been writing the Release Date of an album to the Date tag,
	// and leave the Release Date tag empty.
	legacyMappings := (originalDate != "") &&
		(releaseDate == "") &&
		(date >= originalDate)
	if legacyMappings {
		return originalDate, originalDate, date
	}
	// when there's no Date, first fall back to Original Date, then to Release Date.
	date = cmp.Or(date, originalDate, releaseDate)
	return date, originalDate, releaseDate
}
