package query

import (
	"fmt"

	"github.com/Masterminds/squirrel"
)

// Exists is a domain query predicate. Callers in core/server must use this
// package instead of reaching into persistence SQL helpers (the old DBLink
// leak).
func Exists(subTable string, cond squirrel.Sqlizer) squirrel.Sqlizer {
	return existsCond{subTable: subTable, cond: cond, not: false}
}

func NotExists(subTable string, cond squirrel.Sqlizer) squirrel.Sqlizer {
	return existsCond{subTable: subTable, cond: cond, not: true}
}

type existsCond struct {
	subTable string
	cond     squirrel.Sqlizer
	not      bool
}

func (e existsCond) ToSql() (string, []any, error) {
	sql, args, err := e.cond.ToSql()
	sql = fmt.Sprintf("exists (select 1 from %s where %s)", e.subTable, sql)
	if e.not {
		sql = "not " + sql
	}
	return sql, args, err
}
