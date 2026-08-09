package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Kind struct {
	prefix string
	name   string
}

func (k Kind) String() string {
	return k.name
}

var (
	KindMediaFileArtwork = Kind{"mf", "media_file"}
	KindArtistArtwork    = Kind{"ar", "artist"}
	KindAlbumArtwork     = Kind{"al", "album"}
	KindPlaylistArtwork  = Kind{"pl", "playlist"}
	KindDiscArtwork      = Kind{"dc", "disc"}
	KindRadioArtwork     = Kind{"ra", "radio"}
)

var artworkKindMap = map[string]Kind{
	KindMediaFileArtwork.prefix: KindMediaFileArtwork,
	KindArtistArtwork.prefix:    KindArtistArtwork,
	KindAlbumArtwork.prefix:     KindAlbumArtwork,
	KindPlaylistArtwork.prefix:  KindPlaylistArtwork,
	KindDiscArtwork.prefix:      KindDiscArtwork,
	KindRadioArtwork.prefix:     KindRadioArtwork,
}

type ArtworkID struct {
	Kind       Kind
	ID         string
	LastUpdate time.Time
}

func (id ArtworkID) String() string {
	if id.ID == "" {
		return ""
	}
	lu := id.LastUpdate.Unix()
	if lu < 0 {
		lu = 0
	}

	var result strings.Builder
	result.Grow(len(id.Kind.prefix) + len(id.ID) + 19)
	result.WriteString(id.Kind.prefix)
	result.WriteByte('-')
	result.WriteString(id.ID)
	result.WriteByte('_')
	result.WriteString(strconv.FormatInt(lu, 16))
	return result.String()
}

func NewArtworkID(kind Kind, id string, lastUpdate *time.Time) ArtworkID {
	artID := ArtworkID{kind, id, time.Time{}}
	if lastUpdate != nil {
		artID.LastUpdate = *lastUpdate
	}
	return artID
}

func ParseArtworkID(id string) (ArtworkID, error) {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) != 2 {
		return ArtworkID{}, errors.New("invalid artwork id")
	}
	kind, ok := artworkKindMap[parts[0]]
	if !ok {
		return ArtworkID{}, errors.New("invalid artwork kind")
	}
	parsedID := ArtworkID{
		Kind: kind,
		ID:   parts[1],
	}
	parts = strings.SplitN(parts[1], "_", 2)
	if len(parts) == 2 {
		if parts[1] != "0" {
			lastUpdate, err := strconv.ParseInt(parts[1], 16, 64)
			if err != nil {
				return ArtworkID{}, err
			}
			parsedID.LastUpdate = time.Unix(lastUpdate, 0)
		}
		parsedID.ID = parts[0]
	}
	return parsedID, nil
}

func MustParseArtworkID(id string) ArtworkID {
	artID, err := ParseArtworkID(id)
	if err != nil {
		panic(artID)
	}
	return artID
}

func DiscArtworkID(albumID string, discNumber int) string {
	return fmt.Sprintf("%s:%d", albumID, discNumber)
}

func ParseDiscArtworkID(id string) (albumID string, discNumber int, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", 0, errors.New("invalid disc artwork id")
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid disc number in artwork id: %w", err)
	}
	return parts[0], num, nil
}

func artworkIDFromAlbum(al Album) ArtworkID {
	return ArtworkID{
		Kind:       KindAlbumArtwork,
		ID:         al.ID,
		LastUpdate: al.UpdatedAt,
	}
}

func artworkIDFromMediaFile(mf MediaFile) ArtworkID {
	return ArtworkID{
		Kind:       KindMediaFileArtwork,
		ID:         mf.ID,
		LastUpdate: mf.UpdatedAt,
	}
}

func artworkIDFromPlaylist(pls Playlist) ArtworkID {
	return ArtworkID{
		Kind:       KindPlaylistArtwork,
		ID:         pls.ID,
		LastUpdate: pls.UpdatedAt,
	}
}

func artworkIDFromArtist(ar Artist) ArtworkID {
	return ArtworkID{
		Kind: KindArtistArtwork,
		ID:   ar.ID,
	}
}

func artworkIDFromRadio(r Radio) ArtworkID {
	return ArtworkID{
		Kind:       KindRadioArtwork,
		ID:         r.ID,
		LastUpdate: r.UpdatedAt,
	}
}
