package persistence

import (
	"strings"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// likeSearch implements searchStrategy using LIKE-based SQL filters.
// Used for CJK fallback and punctuation-only fallback when FTS cannot handle the query.
type likeSearch struct {
	filter Sqlizer
}

func (s *likeSearch) ToSql() (string, []any, error) {
	return s.filter.ToSql()
}

func (s *likeSearch) execute(r sqlRepository, sq SelectBuilder, dest any, cfg searchConfig, options model.QueryOptions) error {
	sq = sq.Where(s.filter)
	sq = sq.OrderBy(cfg.OrderBy...)
	return r.queryAll(sq, dest, options)
}

// newLikeSearch creates a LIKE search against core entity columns (CJK, punctuation fallback).
// No minimum length is enforced, since single CJK characters are meaningful words.
// Returns nil when the query produces no searchable tokens.
func newLikeSearch(tableName, query string) searchStrategy {
	filter := likeSearchExpr(tableName, query)
	if filter == nil {
		return nil
	}
	return &likeSearch{filter: filter}
}

func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func literalLike(column, pattern string) Sqlizer {
	return Expr(column+` LIKE ? ESCAPE '\'`, pattern)
}

// likeSearchColumns defines the core columns to search with LIKE queries.
// These are the primary user-visible fields for each entity type.
// Used as a fallback when FTS5 cannot handle the query (e.g., CJK text, punctuation-only input).
var likeSearchColumns = map[string][]string{
	"media_file": {"title", "album", "artist", "album_artist"},
	"album":      {"name", "album_artist"},
	"artist":     {"name"},
}

// likeSearchExpr generates LIKE-based search filters against core columns.
// Each word in the query must match at least one column (AND between words),
// and each word can match any column (OR within a word).
// Used as a fallback when FTS5 cannot handle the query (e.g., CJK text, punctuation-only input).
func likeSearchExpr(tableName string, s string) Sqlizer {
	s = strings.TrimSpace(s)
	if s == "" {
		log.Trace("Search using LIKE backend, query is empty", "table", tableName)
		return nil
	}
	columns, ok := likeSearchColumns[tableName]
	if !ok {
		log.Trace("Search using LIKE backend, couldn't find columns for this table", "table", tableName)
		return nil
	}
	words := strings.Fields(s)
	wordFilters := And{}
	seen := make(map[string]struct{}, len(words))
	for _, word := range words {
		if _, ok := seen[word]; ok {
			continue
		}
		seen[word] = struct{}{}
		literal := escapeLikePattern(word)
		colFilters := Or{}
		for _, col := range columns {
			colFilters = append(colFilters, literalLike(tableName+"."+col, "%"+literal+"%"))
		}
		wordFilters = append(wordFilters, colFilters)
	}
	log.Trace("Search using LIKE backend", "query", wordFilters, "table", tableName)
	return wordFilters
}
