package persistence

import (
	"reflect"
	"testing"

	"github.com/navidrome/navidrome/model"
)

func TestTagsJSONRoundTrip(t *testing.T) {
	t.Parallel()

	want := model.Tags{
		model.TagGenre:    {"Rock & Roll", "Electro-Pop"},
		model.TagMood:     {"Bright \u266b"},
		model.TagGrouping: {`A "quoted" group`},
	}
	got, err := unmarshalTags(marshalTags(want))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected tags: got %#v, want %#v", got, want)
	}
}

func TestUnmarshalTagsRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	if _, err := unmarshalTags(`{"genre":[`); err == nil {
		t.Fatal("expected malformed tags to return an error")
	}
}
