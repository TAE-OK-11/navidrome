package subsonic

import (
	"strings"
	"testing"

	"github.com/navidrome/navidrome/adapters/rustsearch"
)

func TestOrderRustResultsPreservesRelevanceAndDropsMissing(t *testing.T) {
	t.Parallel()

	type value struct {
		id string
	}
	values := []value{{id: "a"}, {id: "b"}, {id: "c"}}
	ordered := orderRustResults([]string{"c", "missing", "a"}, values, func(value value) string {
		return value.id
	})
	if len(ordered) != 2 || ordered[0].id != "c" || ordered[1].id != "a" {
		t.Fatalf("orderRustResults() = %#v", ordered)
	}
}

func TestRustSearchableQueryKeepsBroadQueriesOnSQLite(t *testing.T) {
	t.Parallel()

	for _, query := range []string{"", `""`, "a", "!!!"} {
		if rustSearchableQuery(query) {
			t.Fatalf("rustSearchableQuery(%q) = true", query)
		}
	}
	for _, query := range []string{"ab", "한", "한글", "AC/DC"} {
		if !rustSearchableQuery(query) {
			t.Fatalf("rustSearchableQuery(%q) = false", query)
		}
	}
}

func TestRustSearchPageSupported(t *testing.T) {
	t.Parallel()

	params := &searchParams{songCount: 20, albumCount: 20, artistCount: 20}
	if !rustSearchPageSupported(params) {
		t.Fatal("first search page should use Rust")
	}
	params.songOffset = rustsearch.MaxResults - params.songCount
	if !rustSearchPageSupported(params) {
		t.Fatal("last complete Rust result window should be supported")
	}
	params.songOffset++
	if rustSearchPageSupported(params) {
		t.Fatal("page beyond the Rust result window should use SQLite")
	}
}

func TestGetSearchParamsBoundsWork(t *testing.T) {
	t.Parallel()

	router := &Router{}
	request := newGetRequest(
		"query=test",
		"songCount=-1",
		"albumCount=999999",
		"artistOffset=-10",
	)
	params, err := router.getSearchParams(request)
	if err != nil {
		t.Fatalf("getSearchParams() error = %v", err)
	}
	if params.songCount != 0 || params.albumCount != rustsearch.MaxResults || params.artistOffset != 0 {
		t.Fatalf("getSearchParams() = %#v", params)
	}

	tooLong := newGetRequest("query=" + strings.Repeat("한", maxSearchQueryRunes+1))
	if _, err := router.getSearchParams(tooLong); err == nil {
		t.Fatal("getSearchParams() accepted an oversized query")
	}
}
