package persistence

import (
	"fmt"
	"testing"

	"github.com/Masterminds/squirrel"
)

var (
	benchmarkUpdateSQL  string
	benchmarkUpdateArgs []any
)

func BenchmarkUpdateMapSQL(b *testing.B) {
	values := make(map[string]any, 70)
	for i := range 70 {
		values[fmt.Sprintf("column_%02d", i)] = i
	}
	where := squirrel.Eq{"id": "track-id"}

	b.Run("squirrel", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			query, args, err := squirrel.Update("media_file").SetMap(values).Where(where).ToSql()
			if err != nil {
				b.Fatal(err)
			}
			benchmarkUpdateSQL, benchmarkUpdateArgs = query, args
		}
	})

	b.Run("direct", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			query, args, err := updateMap("media_file", values, where).ToSql()
			if err != nil {
				b.Fatal(err)
			}
			benchmarkUpdateSQL, benchmarkUpdateArgs = query, args
		}
	})
}
