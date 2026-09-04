package integration

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReadLimitedBodyRejectsOversize(t *testing.T) {
	_, err := readLimitedBody(strings.NewReader(strings.Repeat("x", 100)), 10)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected body too large, got %v", err)
	}
}

func TestReadLimitedBodyAllowsWithinLimit(t *testing.T) {
	data, err := readLimitedBody(bytes.NewReader([]byte("hello")), 10)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q", data)
	}
}

func TestBoundedRoundTripperRejectsContentLength(t *testing.T) {
	inner := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			ContentLength: maxArtworkBodyBytes + 1,
			Body:          io.NopCloser(strings.NewReader("x")),
		}, nil
	})
	rt := &boundedRoundTripper{inner: inner}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := rt.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected content-length rejection")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
