package lastfm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/navidrome/navidrome/core/agents"
	"github.com/navidrome/navidrome/core/integration"
	"github.com/navidrome/navidrome/log"
)

const (
	apiBaseUrl = "https://ws.audioscrobbler.com/2.0/"
	// errCodeRateLimit is Last.fm's "rate limit exceeded" (body code, often with HTTP 200).
	errCodeRateLimit = 29
)

type lastFMError struct {
	Code    int
	Message string
}

func (e *lastFMError) Error() string {
	return fmt.Sprintf("last.fm error(%d): %s", e.Code, e.Message)
}

type scrobbleRejectedError struct {
	Code string
	Text string
}

func (e *scrobbleRejectedError) Error() string {
	if e.Text != "" {
		return fmt.Sprintf("last.fm: scrobble rejected (code=%s): %s", e.Code, e.Text)
	}
	return fmt.Sprintf("last.fm: scrobble rejected (code=%s)", e.Code)
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func newClient(apiKey string, secret string, hc httpDoer) *client {
	return &client{apiKey, secret, hc}
}

type client struct {
	apiKey string
	secret string
	hc     httpDoer
}

func (c *client) albumGetInfo(ctx context.Context, name string, artist string, mbid string, lang string) (*Album, error) {
	params := url.Values{}
	params.Add("method", "album.getInfo")
	params.Add("album", name)
	params.Add("artist", artist)
	params.Add("mbid", mbid)
	params.Add("lang", lang)
	response, err := c.makeRequest(ctx, http.MethodGet, params, false)
	if err != nil {
		return nil, err
	}
	return &response.Album, nil
}

func (c *client) artistGetInfo(ctx context.Context, name string, lang string) (*Artist, error) {
	params := url.Values{}
	params.Add("method", "artist.getInfo")
	params.Add("artist", name)
	params.Add("lang", lang)
	response, err := c.makeRequest(ctx, http.MethodGet, params, false)
	if err != nil {
		return nil, err
	}
	return &response.Artist, nil
}

func (c *client) artistGetSimilar(ctx context.Context, name string, limit int) (*SimilarArtists, error) {
	params := url.Values{}
	params.Add("method", "artist.getSimilar")
	params.Add("artist", name)
	params.Add("limit", strconv.Itoa(limit))
	response, err := c.makeRequest(ctx, http.MethodGet, params, false)
	if err != nil {
		return nil, err
	}
	return &response.SimilarArtists, nil
}

func (c *client) artistGetTopTracks(ctx context.Context, name string, limit int) (*TopTracks, error) {
	params := url.Values{}
	params.Add("method", "artist.getTopTracks")
	params.Add("artist", name)
	params.Add("limit", strconv.Itoa(limit))
	response, err := c.makeRequest(ctx, http.MethodGet, params, false)
	if err != nil {
		return nil, err
	}
	return &response.TopTracks, nil
}

func (c *client) trackGetSimilar(ctx context.Context, name, artist string, limit int) (*SimilarTracks, error) {
	params := url.Values{}
	params.Add("method", "track.getSimilar")
	params.Add("track", name)
	params.Add("artist", artist)
	params.Add("limit", strconv.Itoa(limit))
	response, err := c.makeRequest(ctx, http.MethodGet, params, false)
	if err != nil {
		return nil, err
	}
	return &response.SimilarTracks, nil
}

func (c *client) GetToken(ctx context.Context) (string, error) {
	params := url.Values{}
	params.Add("method", "auth.getToken")
	c.sign(ctx, params)
	response, err := c.makeRequest(ctx, http.MethodGet, params, true)
	if err != nil {
		return "", err
	}
	return response.Token, nil
}

func (c *client) getSession(ctx context.Context, token string) (string, error) {
	params := url.Values{}
	params.Add("method", "auth.getSession")
	params.Add("token", token)
	response, err := c.makeRequest(ctx, http.MethodGet, params, true)
	if err != nil {
		return "", err
	}
	return response.Session.Key, nil
}

type ScrobbleInfo struct {
	artist      string
	track       string
	album       string
	trackNumber int
	mbid        string
	duration    int
	albumArtist string
	timestamp   time.Time
}

func (c *client) updateNowPlaying(ctx context.Context, sessionKey string, info ScrobbleInfo) error {
	params := url.Values{}
	params.Add("method", "track.updateNowPlaying")
	params.Add("artist", info.artist)
	params.Add("track", info.track)
	params.Add("album", info.album)
	params.Add("trackNumber", strconv.Itoa(info.trackNumber))
	params.Add("mbid", info.mbid)
	params.Add("duration", strconv.Itoa(info.duration))
	params.Add("albumArtist", info.albumArtist)
	params.Add("sk", sessionKey)
	resp, err := c.makeRequest(ctx, http.MethodPost, params, true)
	if err != nil {
		return err
	}
	if code := resp.NowPlaying.IgnoredMessage.Code; code != "" && code != "0" {
		return &scrobbleRejectedError{
			Code: resp.NowPlaying.IgnoredMessage.Code,
			Text: resp.NowPlaying.IgnoredMessage.Text,
		}
	}
	return nil
}

func (c *client) scrobble(ctx context.Context, sessionKey string, info ScrobbleInfo) error {
	params := url.Values{}
	params.Add("method", "track.scrobble")
	params.Add("timestamp", strconv.FormatInt(info.timestamp.Unix(), 10))
	params.Add("artist", info.artist)
	params.Add("track", info.track)
	params.Add("album", info.album)
	params.Add("trackNumber", strconv.Itoa(info.trackNumber))
	params.Add("mbid", info.mbid)
	params.Add("duration", strconv.Itoa(info.duration))
	params.Add("albumArtist", info.albumArtist)
	params.Add("sk", sessionKey)
	resp, err := c.makeRequest(ctx, http.MethodPost, params, true)
	if err != nil {
		return err
	}
	if resp.Scrobbles.Scrobble.IgnoredMessage.Code != "0" {
		return &scrobbleRejectedError{
			Code: resp.Scrobbles.Scrobble.IgnoredMessage.Code,
			Text: resp.Scrobbles.Scrobble.IgnoredMessage.Text,
		}
	}
	if resp.Scrobbles.Attr.Accepted != 1 {
		return &scrobbleRejectedError{
			Code: resp.Scrobbles.Scrobble.IgnoredMessage.Code,
			Text: resp.Scrobbles.Scrobble.IgnoredMessage.Text,
		}
	}
	return nil
}

func (c *client) makeRequest(ctx context.Context, method string, params url.Values, signed bool) (*Response, error) {
	params.Add("format", "json")
	params.Add("api_key", c.apiKey)

	if signed {
		c.sign(ctx, params)
	}

	var req *http.Request
	var err error
	if method == http.MethodPost {
		body := strings.NewReader(params.Encode())
		req, err = http.NewRequestWithContext(ctx, method, apiBaseUrl, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequestWithContext(ctx, method, apiBaseUrl, nil)
		if err != nil {
			return nil, err
		}
		req.URL.RawQuery = params.Encode()
	}

	log.Trace(ctx, fmt.Sprintf("Sending Last.fm %s request", req.Method), "url", req.URL)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)

	var response Response
	jsonErr := decoder.Decode(&response)
	if resp.StatusCode != 200 && jsonErr != nil {
		return nil, fmt.Errorf("last.fm http status: (%d)", resp.StatusCode)
	}
	if jsonErr != nil {
		return nil, jsonErr
	}
	if response.Error != 0 {
		var err error = &lastFMError{Code: response.Error, Message: response.Message}
		if response.Error == errCodeRateLimit {
			err = errors.Join(err, &agents.RetryLaterError{})
		}
		return &response, err
	}

	return &response, nil
}

func (c *client) sign(ctx context.Context, params url.Values) {
	flat := make(map[string]string, len(params))
	for k, v := range params {
		if k == "format" || k == "callback" || k == "api_sig" || len(v) == 0 {
			continue
		}
		flat[k] = v[0]
	}
	params.Set("api_sig", integration.Sign(ctx, flat, c.secret))
}
