package query

import (
	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils/slice"
)

// ParticipantIDFilter matches rows of table where the artist participates in
// any of the given roles (any role when empty). Semi-joins <table>_artists.
func ParticipantIDFilter(table string, artistID any, roles ...model.Role) Sqlizer {
	return participantIDFilter(table, artistID, false, roles)
}

// NotParticipantIDFilter is the negation of ParticipantIDFilter.
func NotParticipantIDFilter(table string, artistID any, roles ...model.Role) Sqlizer {
	return participantIDFilter(table, artistID, true, roles)
}

func participantIDFilter(table string, artistID any, negate bool, roles []model.Role) Sqlizer {
	sel := squirrel.Select(table + "_id").From(table + "_artists").Where(squirrel.Eq{"artist_id": artistID})
	if len(roles) > 0 {
		sel = sel.Where(squirrel.Eq{"role": slice.Map(roles, func(r model.Role) string { return r.String() })})
	}
	sql, args, _ := sel.ToSql()
	op := " IN ("
	if negate {
		op = " NOT IN ("
	}
	return expr(table+".id"+op+sql+")", args...)
}
