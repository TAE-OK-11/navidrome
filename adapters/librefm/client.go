package librefm

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/navidrome/navidrome/core/agents"
	"github.com/navidrome/navidrome/log"
)

const (
	// errCodeRateLimit is the Audioscrobbler "rate limit exceeded" body code.
	errCodeRateLimit = 29
)

type libreFMError struct {
	Code    int
	Message string
}

func (e *libreFMError) Error() string {
	return fmt.Sprintf("libre.fm error(%d): %s", e.Code, e.Message)
}

type scrobbleRejectedError struct {
	Code string
	Text string
}

func (e *scrobbleRejectedError) Error() string {
	if e.Text != "" {
		return fmt.Sprintf("libre.fm: scrobble rejected (code=%s): %s", e.Code, e.Text)
	}
	return fmt.Sprintf("libre.fm: scrobble rejected (code=%s)", e.Code)
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func newClient(apiKey, secret, baseURL string, hc httpDoer) *client {
	return &client{apiKey: apiKey, secret: secret, baseURL: baseURL, hc: hc}
}

type client struct {
	apiKey  string
	secret  string
	baseURL string
	hc      httpDoer
}

func (c *client) GetToken(ctx context.Context) (string, error) {
	params := url.Values{}
	params.Add("method", "auth.getToken")
	c.sign(params)
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

func (c *client) validateSessionKey(ctx context.Context, sessionKey string) (string, error) {
	params := url.Values{}
	params.Add("method", "user.getInfo")
	params.Add("sk", sessionKey)
	response, err := c.makeRequest(ctx, http.MethodGet, params, true)
	if err != nil {
		return "", err
	}
	if response.User.Name == "" {
		return "", fmt.Errorf("invalid session key")
	}
	return response.User.Name, nil
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
	if resp.NowPlaying.IgnoredMessage.Code != "" && resp.NowPlaying.IgnoredMessage.Code != "0" {
		log.Warn(ctx, "Libre.fm: NowPlaying was ignored", "code", resp.NowPlaying.IgnoredMessage.Code,
			"text", resp.NowPlaying.IgnoredMessage.Text)
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
	if resp.Scrobbles.Scrobble.IgnoredMessage.Code != "" && resp.Scrobbles.Scrobble.IgnoredMessage.Code != "0" {
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
		c.sign(params)
	}

	var req *http.Request
	if method == http.MethodPost {
		body := strings.NewReader(params.Encode())
		req, _ = http.NewRequestWithContext(ctx, method, c.baseURL, body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, _ = http.NewRequestWithContext(ctx, method, c.baseURL, nil)
		req.URL.RawQuery = params.Encode()
	}

	log.Trace(ctx, fmt.Sprintf("Sending Libre.fm %s request", req.Method), "url", req.URL)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)

	var response Response
	jsonErr := decoder.Decode(&response)
	if resp.StatusCode != 200 && jsonErr != nil {
		return nil, fmt.Errorf("libre.fm http status: (%d)", resp.StatusCode)
	}
	if jsonErr != nil {
		return nil, jsonErr
	}
	if response.Error != 0 {
		var err error = &libreFMError{Code: response.Error, Message: response.Message}
		if response.Error == errCodeRateLimit {
			err = errors.Join(err, &agents.RetryLaterError{})
		}
		return &response, err
	}

	return &response, nil
}

func (c *client) sign(params url.Values) {
	keys := make([]string, 0, len(params))
	for k := range params {
		if slices.Contains([]string{"format", "callback"}, k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	msg := strings.Builder{}
	for _, k := range keys {
		msg.WriteString(k)
		msg.WriteString(params[k][0])
	}
	msg.WriteString(c.secret)
	hash := md5.Sum([]byte(msg.String()))
	params.Add("api_sig", hex.EncodeToString(hash[:]))
}
