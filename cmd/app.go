package cmd

import (
	"context"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/navidrome/navidrome/adapters/lastfm"
	"github.com/navidrome/navidrome/adapters/librefm"
	"github.com/navidrome/navidrome/adapters/listenbrainz"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/metrics"
	"github.com/navidrome/navidrome/core/playback"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins"
	"github.com/navidrome/navidrome/scanner"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/server/backgrounds"
	"github.com/navidrome/navidrome/server/nativeapi"
	"github.com/navidrome/navidrome/server/public"
	"github.com/navidrome/navidrome/server/subsonic"
)

// App is the process-wide composition root.
//
// Request path:
//  1. Server global middleware (security, CORS, recover, JWT, logging)
//  2. Mounted router (Native / Subsonic / Public / scrobble auth)
//  3. Router auth + resource middleware
//  4. Shared core services (DataStore, Scanner, streamer, artwork, playlists)
//  5. Handler writes the HTTP response
//
// Background work (startup/periodic/signal scans, folder watcher, jukebox,
// plugins, insights) uses the same service instances as those HTTP handlers.
//
// CreateApp must be called once per server process. The leftover Create*
// injectors exist for isolated CLI commands; constructing them inside the
// server process used to allocate duplicate scanners, transcode limiters, and
// artwork caches so scan status, stream caps, and cache warming diverged.
type App struct {
	Server       *server.Server
	NativeAPI    *nativeapi.Router
	SubsonicAPI  *subsonic.Router
	Public       *public.Router
	LastFM       *lastfm.Router
	ListenBrainz *listenbrainz.Router
	LibreFM      *librefm.Router
	Insights     metrics.Insights
	Prometheus   metrics.Metrics
	Scanner      model.Scanner
	Watcher      scanner.Watcher
	Playback     playback.PlaybackServer
	Plugins      *plugins.Manager
	DataStore    model.DataStore
}

func newApp(
	srv *server.Server,
	nativeAPI *nativeapi.Router,
	subsonicAPI *subsonic.Router,
	publicAPI *public.Router,
	lastFM *lastfm.Router,
	listenBrainz *listenbrainz.Router,
	libreFM *librefm.Router,
	insights metrics.Insights,
	prometheus metrics.Metrics,
	scannerSvc model.Scanner,
	watcher scanner.Watcher,
	playbackSvc playback.PlaybackServer,
	pluginMgr *plugins.Manager,
	ds model.DataStore,
) *App {
	pluginMgr.SetSubsonicRouter(subsonicAPI)
	return &App{
		Server:       srv,
		NativeAPI:    nativeAPI,
		SubsonicAPI:  subsonicAPI,
		Public:       publicAPI,
		LastFM:       lastFM,
		ListenBrainz: listenBrainz,
		LibreFM:      libreFM,
		Insights:     insights,
		Prometheus:   prometheus,
		Scanner:      scannerSvc,
		Watcher:      watcher,
		Playback:     playbackSvc,
		Plugins:      pluginMgr,
		DataStore:    ds,
	}
}

func (a *App) mountRouters(ctx context.Context) {
	a.Server.MountRouter("Native API", consts.URLPathNativeAPI, a.NativeAPI)
	a.Server.MountRouter("Subsonic API", consts.URLPathSubsonicAPI, a.SubsonicAPI)
	a.Server.MountRouter("Public Endpoints", consts.URLPathPublic, a.Public)
	if conf.Server.LastFM.Enabled {
		a.Server.MountRouter("LastFM Auth", consts.URLPathNativeAPI+"/lastfm", a.LastFM)
	}
	if conf.Server.ListenBrainz.Enabled {
		a.Server.MountRouter("ListenBrainz Auth", consts.URLPathNativeAPI+"/listenbrainz", a.ListenBrainz)
	}
	if conf.Server.LibreFM.Enabled {
		a.Server.MountRouter("Libre.fm Auth", consts.URLPathNativeAPI+"/librefm", a.LibreFM)
	}
	if conf.Server.Prometheus.Enabled {
		a.Prometheus.WriteInitialMetrics(ctx)
		a.Server.MountRouter("Prometheus metrics", conf.Server.Prometheus.MetricsPath, a.Prometheus.GetHandler())
	}
	if conf.Server.DevEnableProfiler && profilerAllowedAddress(conf.Server.Address) {
		a.Server.MountRouter("Profiling", "/debug", middleware.Profiler())
	} else if conf.Server.DevEnableProfiler {
		log.Warn("Profiler disabled because the server address is not loopback-only", "address", conf.Server.Address)
	}
	if strings.HasPrefix(conf.Server.UILoginBackgroundURL, "/") {
		a.Server.MountRouter("Background images", conf.Server.UILoginBackgroundURL, backgrounds.NewHandler())
	}
}

func (a *App) runHTTP(ctx context.Context) error {
	a.mountRouters(ctx)
	return a.Server.Run(ctx, conf.Server.Address, conf.Server.Port, conf.Server.TLSCert, conf.Server.TLSKey)
}

// GetPluginManager returns the process plugin manager. CLI plugin commands use
// this so they share the same Subsonic router the manager will call into.
func GetPluginManager(ctx context.Context) *plugins.Manager {
	return CreateApp(ctx).Plugins
}
