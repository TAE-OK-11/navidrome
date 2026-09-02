package apikeys

import (
	"context"
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

func TestDeleteReturnsNotFoundForMissingKey(t *testing.T) {
	ds := &tests.MockDataStore{}
	service := New(ds)

	err := service.Delete(context.Background(), "user-1", "missing")
	if err == nil || err != model.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
