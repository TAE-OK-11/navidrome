// nolint:unused
package scanner

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	ppl "github.com/google/go-pipeline/pkg/pipeline"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/query"
)

const albumRefreshBatchSize = 100

// phaseRefreshAlbums is responsible for refreshing albums that have been
// newly added or changed during the scan process. This phase ensures that
// the album information in the database is up-to-date by performing the
// following steps:
//  1. Loads all libraries and their albums that have been touched (new or changed).
//  2. For each album, it filters out unmodified albums by comparing the current
//     state with the state in the database.
//  3. Refreshes the album information in the database if any changes are detected.
//  4. Logs the results and finalizes the phase by reporting the total number of
//     refreshed and skipped albums.
//  5. As a last step, it refreshes the artist statistics to reflect the changes
type phaseRefreshAlbums struct {
	ds        model.DataStore
	ctx       context.Context
	refreshed atomic.Uint32
	skipped   atomic.Uint32
	state     *scanState
}

func createPhaseRefreshAlbums(ctx context.Context, state *scanState, ds model.DataStore) *phaseRefreshAlbums {
	return &phaseRefreshAlbums{ctx: ctx, ds: ds, state: state}
}

func (p *phaseRefreshAlbums) description() string {
	return "Refresh all new/changed albums"
}

func (p *phaseRefreshAlbums) producer() ppl.Producer[*model.Album] {
	return ppl.NewProducer(p.produce, ppl.Name("load albums from db"))
}

func (p *phaseRefreshAlbums) produce(put func(album *model.Album)) error {
	count := 0
	batch := make([]*model.Album, 0, albumRefreshBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		ids := make([]string, len(batch))
		for i, album := range batch {
			ids[i] = album.ID
		}
		mfs, err := p.ds.MediaFile(p.ctx).GetAll(model.QueryOptions{
			Filters: query.Eq("album_id", ids),
		})
		if err != nil {
			return fmt.Errorf("loading media files for album batch: %w", err)
		}
		byAlbum := make(map[string]model.MediaFiles, len(batch))
		for _, mf := range mfs {
			byAlbum[mf.AlbumID] = append(byAlbum[mf.AlbumID], mf)
		}
		for _, album := range batch {
			if refreshed := p.filterUnmodifiedWithTracks(album, byAlbum[album.ID]); refreshed != nil {
				put(refreshed)
			}
		}
		batch = batch[:0]
		return nil
	}

	for _, lib := range p.state.libraries {
		cursor, err := p.ds.Album(p.ctx).GetTouchedAlbums(lib.ID)
		if err != nil {
			return fmt.Errorf("loading touched albums: %w", err)
		}
		log.Debug(p.ctx, "Scanner: Checking albums that may need refresh", "libraryId", lib.ID, "libraryName", lib.Name)
		for album, err := range cursor {
			if err != nil {
				return fmt.Errorf("loading touched albums: %w", err)
			}
			count++
			batch = append(batch, &album)
			if len(batch) >= albumRefreshBatchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if count == 0 {
		log.Debug(p.ctx, "Scanner: No albums needing refresh")
	} else {
		log.Debug(p.ctx, "Scanner: Found albums that may need refreshing", "count", count)
	}
	return nil
}

func (p *phaseRefreshAlbums) stages() []ppl.Stage[*model.Album] {
	return []ppl.Stage[*model.Album]{
		ppl.NewStage(p.refreshAlbum, ppl.Name("refresh albums")),
	}
}

func (p *phaseRefreshAlbums) filterUnmodifiedWithTracks(album *model.Album, mfs model.MediaFiles) *model.Album {
	if len(mfs) == 0 {
		log.Debug(p.ctx, "Scanner: album has no media files. Skipping", "album_id", album.ID,
			"name", album.Name, "songCount", album.SongCount, "updatedAt", album.UpdatedAt)
		p.skipped.Add(1)
		return nil
	}

	newAlbum := mfs.ToAlbum()
	if album.Equals(newAlbum) {
		log.Trace("Scanner: album is up to date. Skipping", "album_id", album.ID,
			"name", album.Name, "songCount", album.SongCount, "updatedAt", album.UpdatedAt)
		p.skipped.Add(1)
		return nil
	}
	return &newAlbum
}

func (p *phaseRefreshAlbums) refreshAlbum(album *model.Album) (*model.Album, error) {
	if album == nil {
		return nil, nil
	}
	start := time.Now()
	err := p.ds.Album(p.ctx).Put(album)
	log.Debug(p.ctx, "Scanner: refreshing album", "album_id", album.ID, "name", album.Name, "songCount", album.SongCount, "elapsed", time.Since(start), err)
	if err != nil {
		return nil, fmt.Errorf("refreshing album %s: %w", album.ID, err)
	}
	p.refreshed.Add(1)
	p.state.changesDetected.Store(true)
	return album, nil
}

func (p *phaseRefreshAlbums) finalize(err error) error {
	if err != nil {
		return err
	}
	logF := log.Info
	refreshed := p.refreshed.Load()
	skipped := p.skipped.Load()
	if refreshed == 0 {
		logF = log.Debug
	}
	logF(p.ctx, "Scanner: Finished refreshing albums", "refreshed", refreshed, "skipped", skipped, err)
	if !p.state.changesDetected.Load() {
		log.Debug(p.ctx, "Scanner: No changes detected, skipping refreshing annotations")
		return nil
	}
	// Refresh album annotations
	start := time.Now()
	cnt, err := p.ds.Album(p.ctx).RefreshPlayCounts()
	if err != nil {
		return fmt.Errorf("refreshing album annotations: %w", err)
	}
	log.Debug(p.ctx, "Scanner: Refreshed album annotations", "albums", cnt, "elapsed", time.Since(start))

	// Refresh artist annotations
	start = time.Now()
	cnt, err = p.ds.Artist(p.ctx).RefreshPlayCounts()
	if err != nil {
		return fmt.Errorf("refreshing artist annotations: %w", err)
	}
	log.Debug(p.ctx, "Scanner: Refreshed artist annotations", "artists", cnt, "elapsed", time.Since(start))
	p.state.changesDetected.Store(true)
	return nil
}
