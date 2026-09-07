package subsonic

import (
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"net/http/httptest"
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

func TestCatalogCacheKeysSeparateUsersAndRepresentations(t *testing.T) {
	oldShare, oldHost := conf.Server.ShareURL, conf.Server.BaseHost
	conf.Server.ShareURL, conf.Server.BaseHost = "", ""
	t.Cleanup(func() { conf.Server.ShareURL, conf.Server.BaseHost = oldShare, oldHost })
	r := httptest.NewRequest("GET", "https://music.example/rest/getSong", nil)
	user := model.User{ID: "alice", Libraries: model.Libraries{{ID: 1}}}
	r = r.WithContext(request.WithUser(r.Context(), user))
	original := entityResponseCacheKey(r, "song", "id")
	albumKey := albumListCacheKey(r, "starred", []int{1}, 0, 10, "", 0, 0)
	user.ID = "bob"
	other := r.WithContext(request.WithUser(r.Context(), user))
	if entityResponseCacheKey(other, "song", "id") == original {
		t.Fatal("users share rendered entity response")
	}
	if albumListCacheKey(other, "starred", []int{1}, 0, 10, "", 0, 0) == albumKey {
		t.Fatal("users share annotated album lists")
	}
	other = r.WithContext(request.WithClient(r.Context(), "legacy-client"))
	if entityResponseCacheKey(other, "song", "id") == original {
		t.Fatal("clients share incompatible response representations")
	}
	other = r.WithContext(request.WithTranscoding(r.Context(), model.Transcoding{TargetFormat: "opus"}))
	if entityResponseCacheKey(other, "song", "id") == original {
		t.Fatal("transcoding settings omitted from response key")
	}
	other = r.WithContext(request.WithPlayer(r.Context(), model.Player{ReportRealPath: true}))
	if entityResponseCacheKey(other, "song", "id") == original {
		t.Fatal("real and synthetic file paths share a response")
	}
	other = r.Clone(r.Context())
	other.Host = "other.example"
	if entityResponseCacheKey(other, "song", "id") == original {
		t.Fatal("origins share rendered artwork URLs")
	}
	user = model.User{ID: "alice", IsAdmin: true}
	admin := catalogUserKey(request.WithUser(r.Context(), user))
	user.ID = "bob"
	if admin == catalogUserKey(request.WithUser(r.Context(), user)) {
		t.Fatal("admin accounts share private responses")
	}
}
