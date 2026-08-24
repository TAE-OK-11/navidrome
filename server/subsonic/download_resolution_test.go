package subsonic

import (
	"context"
	"errors"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Download archive resolution", func() {
	It("starts with albums after the track lookup has already missed", func() {
		albumRepo := tests.CreateMockAlbumRepo()
		albumRepo.SetData(model.Albums{{ID: "album", Name: "Album"}})
		ds := &tests.MockDataStore{
			MockedAlbum:     albumRepo,
			MockedMediaFile: panicMediaFileRepository{},
		}

		entity, err := downloadArchiveEntityByID(context.Background(), ds, "album")

		Expect(err).NotTo(HaveOccurred())
		Expect(entity).To(Equal(&model.Album{ID: "album", Name: "Album"}))
	})

	It("resolves playlists without probing unsupported radio entities", func() {
		playlistRepo := tests.CreateMockPlaylistRepo()
		playlistRepo.SetData(model.Playlists{{ID: "playlist", Name: "Playlist"}})
		ds := &tests.MockDataStore{
			MockedPlaylist: playlistRepo,
			MockedRadio:    panicRadioRepository{},
		}

		entity, err := downloadArchiveEntityByID(context.Background(), ds, "playlist")

		Expect(err).NotTo(HaveOccurred())
		Expect(entity).To(Equal(&model.Playlist{ID: "playlist", Name: "Playlist"}))
	})

	It("preserves repository failures", func() {
		albumRepo := tests.CreateMockAlbumRepo()
		albumRepo.SetError(true)
		ds := &tests.MockDataStore{MockedAlbum: albumRepo}

		_, err := downloadArchiveEntityByID(context.Background(), ds, "broken")

		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, model.ErrNotFound)).To(BeFalse())
	})
})

type panicMediaFileRepository struct{ model.MediaFileRepository }

func (panicMediaFileRepository) Get(string) (*model.MediaFile, error) {
	panic("download archive resolution repeated the media-file lookup")
}

type panicRadioRepository struct{ model.RadioRepository }

func (panicRadioRepository) Get(string) (*model.Radio, error) {
	panic("download archive resolution probed the radio repository")
}
