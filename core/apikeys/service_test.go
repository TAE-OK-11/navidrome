package apikeys

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

func TestCreateRejectsPastExpiry(t *testing.T) {
	ds := &tests.MockDataStore{}
	service := New(ds)
	past := time.Now().Add(-time.Hour)

	_, _, err := service.Create(context.Background(), "user-1", CreateInput{
		Name:      "test",
		ExpiresAt: &past,
	})
	if err == nil {
		t.Fatal("expected error for past expiresAt")
	}
}

func TestGoTokenRoundTripInTests(t *testing.T) {
	token, prefix, hash, err := generateTokenGo("pepper-value-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || prefix == "" || hash == "" {
		t.Fatalf("empty token fields token=%q prefix=%q hash=%q", token, prefix, hash)
	}
	if !verifyTokenGo(token, hash, "pepper-value-for-tests") {
		t.Fatal("Go hasher must verify its own tokens in tests")
	}
	if verifyTokenGo(token, hash, "other-pepper") {
		t.Fatal("token must not verify with a different pepper")
	}
}

func TestDeleteReturnsNotFoundForMissingKey(t *testing.T) {
	ds := &tests.MockDataStore{}
	service := New(ds)

	err := service.Delete(context.Background(), "user-1", "missing")
	if err == nil || !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
