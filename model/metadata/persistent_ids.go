package metadata

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/navidrome/navidrome/utils/str"
)

type hashFunc = func(...string) string

// computePID calculates the persistent ID for a given spec. The spec is a
// pipe-separated list of fields, where each field is a comma-separated list of
// attributes. Attributes can be either tags or processed values like folder,
// albumid, albumartistid, etc. For each field, it gets all its attribute values
// and concatenates them, then hashes the result. If a field is empty, it is
// skipped and the function looks for the next field.
//
// Taking hash as a parameter (instead of closing over it in a factory) keeps
// mf on the stack: closing over mf would force the whole ~1KB MediaFile to the
// heap on every call.
func computePID(mf model.MediaFile, md Metadata, spec string, prependLibId bool, hash hashFunc) string {
	switch spec {
	case "track_legacy":
		return legacyTrackID(mf, prependLibId)
	case "album_legacy":
		return legacyAlbumID(mf, md, prependLibId)
	}
	pid := ""
	fields := strings.SplitSeq(spec, "|")
	for field := range fields {
		attributes := strings.Split(field, ",")
		values := make([]string, len(attributes))
		hasValue := false
		for i, attr := range attributes {
			v := getPIDAttr(mf, md, attr, prependLibId, spec, hash)
			if v != "" {
				hasValue = true
			}
			values[i] = v
		}
		if hasValue {
			pid += strings.Join(values, "\\")
			break
		}
	}
	if prependLibId {
		pid = fmt.Sprintf("%d\\%s", mf.LibraryID, pid)
	}
	return hash(pid)
}

func getPIDAttr(mf model.MediaFile, md Metadata, attr string, prependLibId bool, spec string, hash hashFunc) string {
	attr = strings.TrimSpace(strings.ToLower(attr))
	switch attr {
	case "albumid":
		if spec == conf.Server.PID.Album {
			log.Error("Recursive PID definition detected, ignoring `albumid`", "spec", spec)
			return ""
		}
		return computePID(mf, md, conf.Server.PID.Album, prependLibId, hash)
	case "folder":
		return filepath.Dir(mf.Path)
	case "albumartistid":
		return hash(str.Clear(strings.ToLower(mf.AlbumArtist)))
	case "title":
		return mf.Title
	case "album":
		album := pidTagValue(mf, md, model.TagAlbum)
		return str.Clear(strings.ToLower(album))
	}
	return pidTagValue(mf, md, model.TagName(attr))
}

func pidTagValue(mf model.MediaFile, md Metadata, tag model.TagName) string {
	if v := md.String(tag); v != "" {
		return v
	}
	if values := mf.Tags[tag]; len(values) > 0 && values[0] != "" {
		return values[0]
	}
	switch tag {
	case model.TagAlbum:
		return mf.Album
	case model.TagTitle:
		return mf.Title
	case model.TagTrackNumber, model.TagName("tracknumber"):
		if mf.TrackNumber > 0 {
			return strconv.Itoa(mf.TrackNumber)
		}
	case model.TagDiscNumber, model.TagName("discnumber"):
		if mf.DiscNumber > 0 {
			return strconv.Itoa(mf.DiscNumber)
		}
	case model.TagMusicBrainzRecordingID:
		return mf.MbzRecordingID
	case model.TagMusicBrainzTrackID:
		return mf.MbzReleaseTrackID
	case model.TagMusicBrainzAlbumID:
		return mf.MbzAlbumID
	case model.TagAlbumVersion:
		return mf.MbzAlbumComment
	case model.TagReleaseDate:
		return mf.ReleaseDate
	}
	return ""
}

func (md Metadata) trackPID(mf model.MediaFile) string {
	return computePID(mf, md, conf.Server.PID.Track, true, id.NewHash)
}

func (md Metadata) albumID(mf model.MediaFile, pidConf string) string {
	return computePID(mf, md, pidConf, true, id.NewHash)
}

// BFR Must be configurable?
func (md Metadata) artistID(name string) string {
	mf := model.MediaFile{AlbumArtist: name}
	return computePID(mf, md, "albumartistid", false, id.NewHash)
}
