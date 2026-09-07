package deezer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/navidrome/navidrome/core/agents"
)

func TestQuotaResponsesAreTemporaryAtAgentBoundary(t *testing.T) {
	for _, endpoint := range []string{"artist", "album"} {
		t.Run(endpoint, func(t *testing.T) {
			client := &fakeHttpClient{}
			client.mock("https://api.deezer.com/search/"+endpoint, http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"Exception","message":"Quota limit exceeded","code":4}}`)),
			})
			agent := &deezerAgent{client: newClient(client)}
			var err error
			if endpoint == "artist" {
				_, err = agent.searchArtist(context.Background(), "Queen")
			} else {
				_, err = agent.searchAlbum(context.Background(), "Jazz", "Queen")
			}
			var retry *agents.RetryLaterError
			if !errors.As(err, &retry) {
				t.Fatalf("quota must remain retryable: %v", err)
			}
			if errors.Is(err, agents.ErrNotFound) {
				t.Fatal("quota became a negative metadata lookup")
			}
		})
	}
}

func TestDeezerNonSuccessAndMalformedResponse(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{200, `{"error":{"code":100,"message":"Invalid parameter"}}`},
		{200, `{"data":`},
		{503, `<html>Unavailable</html>`},
		{429, `{}`},
	} {
		client := &fakeHttpClient{}
		client.mock("https://api.deezer.com/search/artist", http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))})
		_, err := newClient(client).searchArtists(context.Background(), "Queen", 20)
		if err == nil || errors.Is(err, ErrNotFound) {
			t.Fatalf("%d %s: lost provider failure: %v", tc.status, tc.body, err)
		}
	}
}
