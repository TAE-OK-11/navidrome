package query

import (
	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
)

// ItemTags builds indexed tag filters for one item type.
type ItemTags struct {
	IDCol   string
	Table   string
	JoinCol string
}

var (
	SongTags    = ItemTags{IDCol: "media_file.id", Table: "media_file_tags", JoinCol: "media_file_id"}
	AlbumTags   = ItemTags{IDCol: "album.id", Table: "album_tags", JoinCol: "album_id"}
	SongGenres  = SongTags
	AlbumGenres = AlbumTags
)

// ByID matches items tagged with any of the given tag ids (scalar or slice).
func (g ItemTags) ByID(tagIDs any) Sqlizer {
	sub, args, _ := squirrel.Select(g.JoinCol).From(g.Table).Where(squirrel.Eq{"tag_id": tagIDs}).ToSql()
	return expr(g.IDCol+" IN ("+sub+")", args...)
}

// ByTagName matches items by tag name and value through the tag dictionary.
func (g ItemTags) ByTagName(tagName model.TagName, value string) Sqlizer {
	sub, args, _ := squirrel.Select("jt." + g.JoinCol).From(g.Table + " jt").
		Join("tag on tag.id = jt.tag_id").
		Where(squirrel.And{squirrel.Eq{"tag.tag_name": tagName}, squirrel.Like{"tag.tag_value": value}}).ToSql()
	return expr(g.IDCol+" IN ("+sub+")", args...)
}

// ByTagValues matches items tagged with any of the given values for a tag name.
func (g ItemTags) ByTagValues(tagName model.TagName, values []string) Sqlizer {
	sub, args, _ := squirrel.Select("jt." + g.JoinCol).From(g.Table + " jt").
		Join("tag on tag.id = jt.tag_id").
		Where(squirrel.And{squirrel.Eq{"tag.tag_name": tagName}, squirrel.Eq{"tag.tag_value": values}}).ToSql()
	return expr(g.IDCol+" IN ("+sub+")", args...)
}

// ByName matches by genre name (Subsonic passes a name, not an id).
func (g ItemTags) ByName(genre string) Sqlizer {
	return g.ByTagName(model.TagGenre, genre)
}
