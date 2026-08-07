package persistence

import (
	"strings"
	"testing"
)

func TestEscapeLikePatternTreatsWildcardsLiterally(t *testing.T) {
	got := escapeLikePattern(`100%_mix\name`)
	if got != `100\%\_mix\\name` {
		t.Fatalf("unexpected LIKE escaping: %q", got)
	}
}

func TestLikeSearchUsesEscapeClause(t *testing.T) {
	expr := likeSearchExpr("artist", "100%")
	sql, args, err := expr.ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `ESCAPE '\'`) {
		t.Fatalf("LIKE query is missing ESCAPE clause: %s", sql)
	}
	if len(args) == 0 || args[0] != `%100\%%` {
		t.Fatalf("unexpected LIKE argument: %#v", args)
	}
}
