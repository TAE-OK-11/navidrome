package model

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/navidrome/navidrome/core/metadataworker"
)

// ParseLyrics is the single entry point for parsing lyrics. Parsing runs in the
// Rust metadata worker (`--parse-lyrics-worker`); Go only frames requests and
// decodes the returned OpenSubsonic lyrics JSON.
func ParseLyrics(ctx context.Context, suffix, lang string, contents []byte) (LyricList, error) {
	jsonPayload, err := metadataworker.PersistentLyricsWorkers().Parse(ctx, suffix, lang, contents)
	if err != nil {
		return nil, fmt.Errorf("parsing lyrics: %w", err)
	}
	if jsonPayload == "" || jsonPayload == "[]" {
		return nil, nil
	}
	var list LyricList
	if err := json.Unmarshal([]byte(jsonPayload), &list); err != nil {
		return nil, fmt.Errorf("decoding parsed lyrics: %w", err)
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list, nil
}
