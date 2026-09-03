package query

import (
	"github.com/Masterminds/squirrel"
)

// Sqlizer is the persistence-facing predicate type. Domain code should
// construct values with the helpers below instead of importing squirrel.
type Sqlizer = squirrel.Sqlizer

func NotMissing() Sqlizer {
	return squirrel.Eq{"missing": false}
}

func Missing() Sqlizer {
	return squirrel.Eq{"missing": true}
}

func Eq(column string, value any) Sqlizer {
	return squirrel.Eq{column: value}
}

func NotEq(column string, value any) Sqlizer {
	return squirrel.NotEq{column: value}
}

func Like(column string, value any) Sqlizer {
	return squirrel.Like{column: value}
}

func Gt(column string, value any) Sqlizer {
	return squirrel.Gt{column: value}
}

func GtOrEq(column string, value any) Sqlizer {
	return squirrel.GtOrEq{column: value}
}

func LtOrEq(column string, value any) Sqlizer {
	return squirrel.LtOrEq{column: value}
}

func ColumnAfter(column string, value any) Sqlizer {
	return squirrel.Gt{column: value}
}

func Or(parts ...Sqlizer) Sqlizer {
	return squirrel.Or(compact(parts))
}

func And(parts ...Sqlizer) Sqlizer {
	return squirrel.And(compact(parts))
}

func compact(parts []Sqlizer) []Sqlizer {
	out := make([]Sqlizer, 0, len(parts))
	for _, part := range parts {
		if part != nil {
			out = append(out, part)
		}
	}
	return out
}
