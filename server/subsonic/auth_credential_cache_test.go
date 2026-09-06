package subsonic

import (
	"testing"
	"time"
)

func TestAuthCredentialCacheRemembersSuccess(t *testing.T) {
	cache := newAuthCredentialCache(8, 40*time.Millisecond)
	key := authCredentialCacheKey("alice", "", "token", "salt", "")
	now := time.Now()
	if cache.seen(key, now) {
		t.Fatal("empty cache should miss")
	}
	cache.remember(key, now)
	if !cache.seen(key, now.Add(10*time.Millisecond)) {
		t.Fatal("expected credential cache hit")
	}
	if cache.seen(key, now.Add(50*time.Millisecond)) {
		t.Fatal("expired credential cache entry should miss")
	}
}

func TestAuthCredentialCacheKeyDistinguishesAuthModes(t *testing.T) {
	passKey := authCredentialCacheKey("bob", "secret", "", "", "")
	tokenKey := authCredentialCacheKey("bob", "", "tok", "salt", "")
	jwtKey := authCredentialCacheKey("bob", "", "", "", "jwt-token")
	if passKey == tokenKey || passKey == jwtKey || tokenKey == jwtKey {
		t.Fatalf("credential cache keys collided: %q %q %q", passKey, tokenKey, jwtKey)
	}
}
