package persistence

import (
	"testing"

	"github.com/navidrome/navidrome/model"
)

func TestUnmarshalParticipants(t *testing.T) {
	t.Parallel()

	data := `{"artist":[{"id":"artist-1","name":"Beyonce \u266b"}],` +
		`"performer":[{"id":"artist-2","name":"A \"quoted\" name","subRole":"guitar"}]}`
	got, err := unmarshalParticipants(data)
	if err != nil {
		t.Fatal(err)
	}
	if got[model.RoleArtist][0].Name != "Beyonce \u266b" {
		t.Fatalf("unexpected Unicode artist name: %q", got[model.RoleArtist][0].Name)
	}
	performer := got[model.RolePerformer][0]
	if performer.ID != "artist-2" || performer.Name != `A "quoted" name` || performer.SubRole != "guitar" {
		t.Fatalf("unexpected performer: %#v", performer)
	}
}

func TestUnmarshalParticipantsRejectsInvalidData(t *testing.T) {
	t.Parallel()

	for _, data := range []string{
		`{"artist":[`,
		`{"not-a-role":[{"id":"1","name":"Artist"}]}`,
	} {
		if _, err := unmarshalParticipants(data); err == nil {
			t.Fatalf("expected error for %q", data)
		}
	}
}

func TestUnmarshalParticipantsReturnsIndependentValues(t *testing.T) {
	t.Parallel()

	data := marshalParticipants(model.Participants{
		model.RoleArtist: {{Artist: model.Artist{ID: "artist-1", Name: "Artist"}}},
	})
	first, err := unmarshalParticipants(data)
	if err != nil {
		t.Fatal(err)
	}
	first[model.RoleArtist][0].Name = "Changed"
	first[model.RoleComposer] = model.ParticipantList{{Artist: model.Artist{ID: "composer-1"}}}

	second, err := unmarshalParticipants(data)
	if err != nil {
		t.Fatal(err)
	}
	if second[model.RoleArtist][0].Name != "Artist" {
		t.Fatalf("cached participant was mutated: %#v", second[model.RoleArtist][0])
	}
	if _, ok := second[model.RoleComposer]; ok {
		t.Fatal("cached participant map was mutated")
	}
}
