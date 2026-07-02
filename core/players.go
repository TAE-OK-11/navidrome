package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/utils"
)

type Players interface {
	Get(ctx context.Context, playerId string) (*model.Player, error)
	Register(ctx context.Context, id, client, userAgent, ip string) (*model.Player, *model.Transcoding, error)
	RegisterFresh(ctx context.Context, id, client, userAgent, ip string) (*model.Player, *model.Transcoding, error)
}

func NewPlayers(ds model.DataStore) Players {
	return &players{
		ds:      ds,
		limiter: utils.Limiter{Interval: consts.UpdatePlayerFrequency},
	}
}

type players struct {
	ds      model.DataStore
	limiter utils.Limiter
	cache   playerLookupCache
}

const playerLookupCacheTTL = time.Second

type playerLookupCache struct {
	byID            sync.Map
	byMatch         sync.Map
	lastSweepUnixNs atomic.Int64
}

type playerCacheEntry struct {
	player        model.Player
	transcoding   model.Transcoding
	hasTranscode  bool
	expires       time.Time
	matchCacheKey string
}

func (p *players) Register(ctx context.Context, playerID, client, userAgent, ip string) (*model.Player, *model.Transcoding, error) {
	return p.register(ctx, playerID, client, userAgent, ip, true)
}

func (p *players) RegisterFresh(ctx context.Context, playerID, client, userAgent, ip string) (*model.Player, *model.Transcoding, error) {
	return p.register(ctx, playerID, client, userAgent, ip, false)
}

func (p *players) register(ctx context.Context, playerID, client, userAgent, ip string, useCache bool) (*model.Player, *model.Transcoding, error) {
	var plr *model.Player
	var trc *model.Transcoding
	var err error
	user, _ := request.UserFrom(ctx)
	username := userName(ctx)
	matchKey := playerMatchCacheKey(user.ID, client, userAgent)
	if playerID != "" {
		if useCache {
			if plr, trc, ok := p.cache.getByID(playerID, client); ok {
				err = p.updatePlayer(ctx, plr, client, userAgent, ip, username)
				return plr, trc, err
			}
		}

		plr, err = p.ds.Player(ctx).Get(playerID)
		if err == nil {
			if plr.Client != client {
				p.cache.delete(playerID, matchKey)
				playerID = ""
			}
		}
	}
	if err != nil || playerID == "" {
		if useCache {
			if plr, trc, ok := p.cache.getByMatch(matchKey); ok {
				err = p.updatePlayer(ctx, plr, client, userAgent, ip, username)
				return plr, trc, err
			}
		}

		plr, err = p.ds.Player(ctx).FindMatch(user.ID, client, userAgent)
		switch {
		case err == nil:
			log.Debug(ctx, "Found matching player", "id", plr.ID, "client", client, "username", username, "type", userAgent)
		default:
			if !errors.Is(err, model.ErrNotFound) {
				return nil, nil, err
			}
			plr = newPlayer(user.ID, client)
			log.Info(ctx, "Registering new player", "id", plr.ID, "client", client, "username", username, "type", userAgent)
		}
	}
	err = p.updatePlayer(ctx, plr, client, userAgent, ip, username)
	if err != nil {
		return plr, nil, err
	}
	if plr.TranscodingId != "" {
		trc, err = p.ds.Transcoding(ctx).Get(plr.TranscodingId)
	}
	if err == nil && useCache {
		p.cache.put(plr.ID, matchKey, plr, trc)
	}
	return plr, trc, err
}

func newPlayer(userID, client string) *model.Player {
	return &model.Player{
		ID:              id.NewRandom(),
		UserId:          userID,
		Client:          client,
		ScrobbleEnabled: true,
		ReportRealPath:  conf.Server.Subsonic.DefaultReportRealPath,
	}
}

func (p *players) updatePlayer(ctx context.Context, plr *model.Player, client, userAgent, ip, username string) error {
	plr.Name = fmt.Sprintf("%s [%s]", client, userAgent)
	plr.UserAgent = userAgent
	plr.IP = ip
	plr.LastSeen = time.Now()
	var err error
	p.limiter.Do(plr.ID, func() {
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		err = p.ds.Player(ctx).Put(plr)
		if err != nil {
			log.Warn(ctx, "Could not save player", "id", plr.ID, "client", client, "username", username, "type", userAgent, err)
		}
	})
	return err
}

func (p *players) Get(ctx context.Context, playerId string) (*model.Player, error) {
	return p.ds.Player(ctx).Get(playerId)
}

func playerMatchCacheKey(userID, client, userAgent string) string {
	return userID + "\x00" + client + "\x00" + userAgent
}

func (c *playerLookupCache) getByID(playerID, client string) (*model.Player, *model.Transcoding, bool) {
	value, ok := c.byID.Load(playerID)
	if !ok {
		return nil, nil, false
	}
	return c.playerFromEntry(value, client, time.Now())
}

func (c *playerLookupCache) getByMatch(matchKey string) (*model.Player, *model.Transcoding, bool) {
	value, ok := c.byMatch.Load(matchKey)
	if !ok {
		return nil, nil, false
	}
	return c.playerFromEntry(value, "", time.Now())
}

func (c *playerLookupCache) playerFromEntry(value any, client string, now time.Time) (*model.Player, *model.Transcoding, bool) {
	entry := value.(playerCacheEntry)
	if now.After(entry.expires) || client != "" && entry.player.Client != client {
		c.delete(entry.player.ID, entry.matchCacheKey)
		return nil, nil, false
	}

	plr := entry.player
	var trc *model.Transcoding
	if entry.hasTranscode {
		transcoding := entry.transcoding
		trc = &transcoding
	}
	return &plr, trc, true
}

func (c *playerLookupCache) put(playerID, matchKey string, plr *model.Player, trc *model.Transcoding) {
	now := time.Now()
	c.sweep(now)
	entry := playerCacheEntry{
		player:        *plr,
		expires:       now.Add(playerLookupCacheTTL),
		matchCacheKey: matchKey,
	}
	if trc != nil {
		entry.transcoding = *trc
		entry.hasTranscode = true
	}
	c.byID.Store(playerID, entry)
	c.byMatch.Store(matchKey, entry)
}

func (c *playerLookupCache) delete(playerID, matchKey string) {
	if playerID != "" {
		c.byID.Delete(playerID)
	}
	if matchKey != "" {
		c.byMatch.Delete(matchKey)
	}
}

func (c *playerLookupCache) sweep(now time.Time) {
	last := c.lastSweepUnixNs.Load()
	if last != 0 && now.Sub(time.Unix(0, last)) < playerLookupCacheTTL {
		return
	}
	if !c.lastSweepUnixNs.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	c.byID.Range(func(_, value any) bool {
		entry := value.(playerCacheEntry)
		if now.After(entry.expires) {
			c.delete(entry.player.ID, entry.matchCacheKey)
		}
		return true
	})
}
