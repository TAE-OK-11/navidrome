package query

import (
	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
)

// Sqlizer is the domain predicate type. Helpers below wrap squirrel so callers
// never import it and never receive a squirrel concrete type.
type Sqlizer = model.Sqlizer

type wrapped struct {
	inner squirrel.Sqlizer
}

func (w wrapped) ToSql() (string, []any, error) {
	if w.inner == nil {
		return "", nil, nil
	}
	return w.inner.ToSql()
}

func wrap(inner squirrel.Sqlizer) Sqlizer {
	if inner == nil {
		return nil
	}
	return wrapped{inner: inner}
}

func expr(sql string, args ...any) Sqlizer {
	return wrap(squirrel.Expr(sql, args...))
}

func NotMissing() Sqlizer {
	return Eq("missing", false)
}

func Missing() Sqlizer {
	return Eq("missing", true)
}

func Eq(column string, value any) Sqlizer {
	return wrap(squirrel.Eq{column: value})
}

func NotEq(column string, value any) Sqlizer {
	return wrap(squirrel.NotEq{column: value})
}

func Like(column string, value any) Sqlizer {
	return wrap(squirrel.Like{column: value})
}

func Gt(column string, value any) Sqlizer {
	return wrap(squirrel.Gt{column: value})
}

func GtOrEq(column string, value any) Sqlizer {
	return wrap(squirrel.GtOrEq{column: value})
}

func LtOrEq(column string, value any) Sqlizer {
	return wrap(squirrel.LtOrEq{column: value})
}

func ColumnAfter(column string, value any) Sqlizer {
	return Gt(column, value)
}

func Or(parts ...Sqlizer) Sqlizer {
	return wrap(squirrel.Or(toSquirrel(compact(parts))))
}

func And(parts ...Sqlizer) Sqlizer {
	return wrap(squirrel.And(toSquirrel(compact(parts))))
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

func toSquirrel(parts []Sqlizer) []squirrel.Sqlizer {
	out := make([]squirrel.Sqlizer, 0, len(parts))
	for _, part := range parts {
		out = append(out, part)
	}
	return out
}
