package subsonic

import (
	"testing"
	"time"

	"github.com/navidrome/navidrome/server/subsonic/responses"
)

func TestEntityResponseCacheDeleteByEntityID(t *testing.T) {
	cache := &entityResponseCache{}
	now := time.Now()
	cache.put("user|artist|abc", now, newResponse())
	cache.put("user|album|xyz", now, newResponse())
	cache.put("user|playlists", now, &responses.Subsonic{})

	cache.deleteByEntityID("abc")
	if _, hit := cache.get("user|artist|abc", now); hit {
		t.Fatal("expected artist cache entry to be deleted")
	}
	if _, hit := cache.get("user|album|xyz", now); !hit {
		t.Fatal("expected unrelated album cache entry to remain")
	}
	if _, hit := cache.get("user|playlists", now); !hit {
		t.Fatal("expected playlists cache entry to remain")
	}
}

func TestEntityResponseCacheDeleteBySuffix(t *testing.T) {
	cache := &entityResponseCache{}
	now := time.Now()
	cache.put("alice|playlists", now, &responses.Subsonic{})
	cache.put("bob|playlists", now, &responses.Subsonic{})
	cache.put("alice|song|1", now, newResponse())

	cache.deleteBySuffix("|playlists")
	if _, hit := cache.get("alice|playlists", now); hit {
		t.Fatal("expected alice playlists cache to be deleted")
	}
	if _, hit := cache.get("bob|playlists", now); hit {
		t.Fatal("expected bob playlists cache to be deleted")
	}
	if _, hit := cache.get("alice|song|1", now); !hit {
		t.Fatal("expected song cache entry to remain")
	}
}
