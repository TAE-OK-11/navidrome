package subsonic

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

func TestInvokePing(t *testing.T) {
	ds := &tests.MockDataStore{}
	userRepo := tests.CreateMockUserRepo()
	ds.MockedUser = userRepo
	_ = userRepo.Put(&model.User{ID: "1", UserName: "foo"})
	api := New(ds, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	ct, body, err := api.Invoke(context.Background(), "ping", url.Values{"u": []string{"foo"}}, "foo", true)
	if err != nil {
		t.Fatal(err)
	}
	if ct != "application/json" {
		t.Fatalf("content-type %s", ct)
	}
	var wrapped map[string]any
	if err := json.Unmarshal(body, &wrapped); err != nil {
		t.Fatal(err)
	}
	resp, ok := wrapped["subsonic-response"].(map[string]any)
	if !ok {
		t.Fatalf("payload %#v", wrapped)
	}
	if resp["status"] != "ok" {
		t.Fatalf("status %#v", resp["status"])
	}
}
