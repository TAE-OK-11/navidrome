package librefm

import (
	"context"
	"errors"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/agents"
	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils/cache"
	"github.com/navidrome/navidrome/utils/httpclient"
)

const (
	libreFMAgentName   = "librefm"
	sessionKeyProperty = "LibreFMSessionKey"
)

type librefmAgent struct {
	ds          model.DataStore
	sessionKeys *agents.SessionKeys
	client      *client
}

func librefmConstructor(ds model.DataStore) *librefmAgent {
	if !conf.Server.LibreFM.Enabled {
		return nil
	}
	l := &librefmAgent{
		ds:          ds,
		sessionKeys: &agents.SessionKeys{DataStore: ds, KeyName: sessionKeyProperty},
	}
	hc := httpclient.New(consts.DefaultHttpClientTimeOut)
	chc := cache.NewHTTPClient(hc, consts.DefaultHttpClientTimeOut)
	l.client = newClient(effectiveApiKey(), effectiveSecret(), conf.Server.LibreFM.BaseURL, chc)
	return l
}

func (l *librefmAgent) AgentName() string {
	return libreFMAgentName
}

func (l *librefmAgent) getArtistForScrobble(track *model.MediaFile, role model.Role, displayName string) string {
	if conf.Server.LibreFM.ScrobbleFirstArtistOnly && len(track.Participants[role]) > 0 {
		return track.Participants[role][0].Name
	}
	return displayName
}

func (l *librefmAgent) NowPlaying(ctx context.Context, userId string, track *model.MediaFile, position int) error {
	sk, err := l.sessionKeys.Get(ctx, userId)
	if err != nil || sk == "" {
		return scrobbler.ErrNotAuthorized
	}

	err = l.client.updateNowPlaying(ctx, sk, ScrobbleInfo{
		artist:      l.getArtistForScrobble(track, model.RoleArtist, track.Artist),
		track:       track.Title,
		album:       track.Album,
		trackNumber: track.TrackNumber,
		mbid:        track.MbzRecordingID,
		duration:    int(track.Duration),
		albumArtist: l.getArtistForScrobble(track, model.RoleAlbumArtist, track.AlbumArtist),
	})
	if err != nil {
		log.Warn(ctx, "Libre.fm client.updateNowPlaying returned error", "track", track.Title, err)
		return errors.Join(err, scrobbler.ErrUnrecoverable)
	}
	return nil
}

func (l *librefmAgent) Scrobble(ctx context.Context, userId string, s scrobbler.Scrobble) error {
	sk, err := l.sessionKeys.Get(ctx, userId)
	if err != nil || sk == "" {
		return errors.Join(err, scrobbler.ErrNotAuthorized)
	}

	if s.Duration <= 30 {
		log.Debug(ctx, "Skipping Libre.fm scrobble for short song", "track", s.Title, "duration", s.Duration)
		return nil
	}
	err = l.client.scrobble(ctx, sk, ScrobbleInfo{
		artist:      l.getArtistForScrobble(&s.MediaFile, model.RoleArtist, s.Artist),
		track:       s.Title,
		album:       s.Album,
		trackNumber: s.TrackNumber,
		mbid:        s.MbzRecordingID,
		duration:    int(s.Duration),
		albumArtist: l.getArtistForScrobble(&s.MediaFile, model.RoleAlbumArtist, s.AlbumArtist),
		timestamp:   s.TimeStamp,
	})
	if err == nil {
		return nil
	}
	var rejected *scrobbleRejectedError
	if errors.As(err, &rejected) {
		log.Warn(ctx, "Libre.fm scrobble rejected by service", "track", s.Title, err)
		return errors.Join(err, scrobbler.ErrUnrecoverable)
	}
	var lfErr *libreFMError
	if errors.As(err, &lfErr) {
		if lfErr.Code == 11 || lfErr.Code == 16 || errors.Is(err, scrobbler.ErrRetryLater) {
			return errors.Join(err, scrobbler.ErrRetryLater)
		}
		return errors.Join(err, scrobbler.ErrUnrecoverable)
	}
	log.Warn(ctx, "Libre.fm client.scrobble returned error", "track", s.Title, err)
	return errors.Join(err, scrobbler.ErrRetryLater)
}

func (l *librefmAgent) IsAuthorized(ctx context.Context, userId string) bool {
	sk, err := l.sessionKeys.Get(ctx, userId)
	return err == nil && sk != ""
}

func (l *librefmAgent) PlaybackReport(context.Context, scrobbler.PlaybackSession) error {
	return nil
}

func init() {
	conf.AddHook(func() {
		if conf.Server.LibreFM.Enabled {
			scrobbler.Register(libreFMAgentName, func(ds model.DataStore) scrobbler.Scrobbler {
				a := librefmConstructor(ds)
				if a != nil {
					return a
				}
				return nil
			})
		}
	})
}
