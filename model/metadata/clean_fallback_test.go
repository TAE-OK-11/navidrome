package metadata

import (
	"testing"

	"github.com/navidrome/navidrome/model"
)

func TestNewWithTagCleanKeepsRustTagsWhenFallbackDisabled(t *testing.T) {
	md := newWithTagClean("song.mp3", Info{
		Tags:        model.RawTags{"TITLE": []string{"ignored"}},
		CleanedTags: model.Tags{model.TagTitle: []string{"FromRust"}},
	}, false)
	if got := md.String(model.TagTitle); got != "FromRust" {
		t.Fatalf("Rust cleaned tags: title=%q, want FromRust", got)
	}
}

func TestNewWithTagCleanEmptyWhenFallbackDisabled(t *testing.T) {
	md := newWithTagClean("song.mp3", Info{
		Tags: model.RawTags{"TITLE": []string{"Hello"}},
	}, false)
	if got := md.String(model.TagTitle); got != "" {
		t.Fatalf("production without Rust tags: title=%q, want empty", got)
	}
}
