package persistence

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
)

var benchmarkLibrariesJSON = func() string {
	now := time.Unix(1_700_000_000, 0).UTC()
	libraries := make(model.Libraries, 4)
	for i := range libraries {
		libraries[i] = model.Library{
			ID:                i + 1,
			Name:              "Library",
			Path:              "/music/library",
			RemotePath:        "s3://bucket/library",
			LastScanAt:        now,
			LastScanStartedAt: now,
			UpdatedAt:         now,
			CreatedAt:         now,
		}
	}
	data, _ := json.Marshal(libraries)
	return string(data)
}()

func BenchmarkDBUserPostScan(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		user := dbUser{User: &model.User{}, LibrariesJSON: benchmarkLibrariesJSON}
		if err := user.PostScan(); err != nil {
			b.Fatal(err)
		}
	}
}
