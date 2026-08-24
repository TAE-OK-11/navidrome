package subsonic

import "testing"

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

	for _, query := range []string{"", `""`, "a", "한", "!!!"} {
		if rustSearchableQuery(query) {
			t.Fatalf("rustSearchableQuery(%q) = true", query)
		}
	}
	for _, query := range []string{"ab", "한글", "AC/DC"} {
		if !rustSearchableQuery(query) {
			t.Fatalf("rustSearchableQuery(%q) = false", query)
		}
	}
}
