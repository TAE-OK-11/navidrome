package filter

import (
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/query"
)

type Options = model.QueryOptions

func addDefaultFilters(options Options) Options {
	if options.Filters == nil {
		options.Filters = query.NotMissing()
	} else {
		options.Filters = query.And(query.NotMissing(), options.Filters)
	}
	return options
}

func AlbumsByNewest() Options {
	return addDefaultFilters(addDefaultFilters(Options{Sort: "recently_added", Order: "desc"}))
}

func AlbumsByRecent() Options {
	return addDefaultFilters(Options{Sort: "playDate", Order: "desc", Filters: query.Gt("play_date", time.Time{})})
}

func AlbumsByFrequent() Options {
	return addDefaultFilters(Options{Sort: "playCount", Order: "desc", Filters: query.Gt("play_count", 0)})
}

func AlbumsByRandom() Options {
	return addDefaultFilters(Options{Sort: "random"})
}

func AlbumsByName() Options {
	return addDefaultFilters(Options{Sort: "name"})
}

func AlbumsByArtist() Options {
	return addDefaultFilters(Options{Sort: "artist"})
}

func AlbumsByArtistID(artistId string) Options {
	roles := []model.Role{model.RoleAlbumArtist}
	if conf.Server.Subsonic.ArtistParticipations {
		roles = append(roles, model.RoleArtist)
	}
	return addDefaultFilters(Options{
		Sort:    "max_year",
		Filters: query.ParticipantIDFilter("album", artistId, roles...),
	})
}

// AlbumsByContributingArtistID matches albums where the artist performs on a track but is not the
// album artist. The disjoint complement of AlbumsByArtistID, so an artist's own discography never
// leaks into it.
func AlbumsByContributingArtistID(artistId string) Options {
	return addDefaultFilters(Options{
		Sort: "max_year",
		Filters: query.And(
			query.ParticipantIDFilter("album", artistId, model.RoleArtist),
			query.NotParticipantIDFilter("album", artistId, model.RoleAlbumArtist),
		),
	})
}

func AlbumsByYear(fromYear, toYear int) Options {
	orderOption := ""
	if fromYear > toYear {
		fromYear, toYear = toYear, fromYear
		orderOption = "desc"
	}
	return addDefaultFilters(Options{
		Sort:  "max_year",
		Order: orderOption,
		Filters: query.Or(
			query.And(
				query.GtOrEq("min_year", fromYear),
				query.LtOrEq("min_year", toYear),
			),
			query.And(
				query.GtOrEq("max_year", fromYear),
				query.LtOrEq("max_year", toYear),
			),
		),
	})
}

func SongsByAlbum(albumId string) Options {
	return addDefaultFilters(Options{
		Filters:            query.Eq("album_id", albumId),
		Sort:               "album",
		ExcludeHeavyFields: true,
	})
}

// SongsByArtistID matches media files where the artist participates as album or track artist, in
// album order.
func SongsByArtistID(artistId string) Options {
	return addDefaultFilters(Options{
		Sort:               "album",
		Filters:            query.ParticipantIDFilter("media_file", artistId, model.RoleArtist, model.RoleAlbumArtist),
		ExcludeHeavyFields: true,
	})
}

func SongsByGenreAndYearRange(genre string, fromYear, toYear int) Options {
	options := Options{}
	var ff []query.Sqlizer
	if genre != "" {
		ff = append(ff, query.SongGenres.ByName(genre))
	}
	if fromYear != 0 {
		ff = append(ff, query.GtOrEq("year", fromYear))
	}
	if toYear != 0 {
		ff = append(ff, query.LtOrEq("year", toYear))
	}
	options.Filters = query.And(ff...)
	options.ExcludeHeavyFields = true
	return addDefaultFilters(options)
}

func ApplyLibraryFilter(opts Options, musicFolderIds []int) Options {
	if len(musicFolderIds) == 0 {
		return opts
	}

	libraryFilter := query.Eq("library_id", musicFolderIds)
	if opts.Filters == nil {
		opts.Filters = libraryFilter
	} else {
		opts.Filters = query.And(opts.Filters, libraryFilter)
	}

	return opts
}

// ApplyArtistLibraryFilter applies a filter to the given Options to ensure that only artists
// that are associated with the specified music folders are included in the results.
func ApplyArtistLibraryFilter(opts Options, musicFolderIds []int) Options {
	if len(musicFolderIds) == 0 {
		return opts
	}

	artistLibraryFilter := query.Eq("library_artist.library_id", musicFolderIds)
	if opts.Filters == nil {
		opts.Filters = artistLibraryFilter
	} else {
		opts.Filters = query.And(opts.Filters, artistLibraryFilter)
	}

	return opts
}

func AlbumsByGenre(genre string) Options {
	return addDefaultFilters(Options{Sort: "name", Filters: query.AlbumGenres.ByName(genre)})
}

func SongsByGenre(genre string) Options {
	return addDefaultFilters(Options{
		Sort:               "name",
		Filters:            query.SongGenres.ByName(genre),
		ExcludeHeavyFields: true,
	})
}

func ByRating() Options {
	return addDefaultFilters(Options{Sort: "rating", Order: "desc", Filters: query.Gt("rating", 0)})
}

func ByStarred() Options {
	return addDefaultFilters(Options{Sort: "starred_at", Order: "desc", Filters: query.Eq("starred", true)})
}

func ArtistsByStarred() Options {
	return Options{Sort: "starred_at", Order: "desc", Filters: query.Eq("starred", true)}
}
