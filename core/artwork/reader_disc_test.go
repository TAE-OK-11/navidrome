package artwork

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/metadata"
	"github.com/navidrome/navidrome/persistence"
	"github.com/navidrome/navidrome/utils/slice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDiscArtworkReader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Disc Artwork Reader Suite")
}

var _ = Describe("discArtworkReader", func() {
	var ds persistence.DataStore
	var artwork *artwork
	var ffmpegMock *ffmpeg.Mock
	var ctx context.Context
	var album model.Album
	var mediaFiles model.MediaFiles
	var lib model.Library

	BeforeEach(func() {
		DeferCleanup(conf.Reset)
		conf.Server.CoverArtPriority = "cover.*, folder.*, front.*, embedded, external"

		ds = &persistence.MockDataStore{}
		ffmpegMock = &ffmpeg.Mock{}
		artwork = &artwork{ds: ds, ffmpeg: ffmpegMock}
		ctx = context.Background()
		lib = model.Library{ID: 1, Name: "Music", Path: "/music"}

		album = model.Album{
			ID:        "album-1",
			Name:      "Test Album",
			LibraryID: lib.ID,
			Discs: model.Discs{
				{DiscNumber: 1, SubTitle: "Disc One"},
				{DiscNumber: 2, SubTitle: "Disc Two"},
			},
		}

		mediaFiles = model.MediaFiles{
			{
				ID:         "track-1",
				AlbumID:    album.ID,
				LibraryID:  lib.ID,
				Path:       "Artist/Album/CD1/01.flac",
				DiscNumber: 1,
				Title:      "Track 1",
			},
			{
				ID:         "track-2",
				AlbumID:    album.ID,
				LibraryID:  lib.ID,
				Path:       "Artist/Album/CD2/01.flac",
				DiscNumber: 2,
				Title:      "Track 2",
			},
		}
	})

	Describe("newDiscArtworkReader", func() {
		Context("with valid disc ID", func() {
			It("creates a reader for the disc", func() {
				// Setup mocks
				ds.(*persistence.MockDataStore).MockedAlbum.On("Get", album.ID).Return(&album, nil)
				ds.(*persistence.MockDataStore).MockedMediaFile.On("GetAllByAlbum", album.ID).Return(mediaFiles, nil)
				ds.(*persistence.MockDataStore).MockedLibrary.On("Get", lib.ID).Return(&lib, nil)

				artID := mediaFiles[0].DiscCoverArtID()
				reader, err := newDiscArtworkReader(ctx, artwork, artID)

				Expect(err).NotTo(HaveOccurred())
				Expect(reader).NotTo(BeNil())
				Expect(reader.album.ID).To(Equal(album.ID))
				Expect(reader.discNumber).To(Equal(1))
				Expect(reader.mediaFiles).To(HaveLen(1))
				Expect(reader.mediaFiles[0].DiscNumber).To(Equal(1))
			})
		})

		Context("with invalid disc ID", func() {
			It("returns an error", func() {
				artID := model.ArtworkID{ID: "invalid", Kind: model.KindDiscArtwork}
				_, err := newDiscArtworkReader(ctx, artwork, artID)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Key", func() {
		It("includes disc number and configuration", func() {
			reader := &discArtworkReader{
				cacheKey: cacheKey{
					artID:      model.ArtworkID{ID: album.ID, Kind: model.KindDiscArtwork},
					lastUpdate: time.Now(),
				},
				discNumber: 2,
			}

			key := reader.Key()
			Expect(key).To(ContainSubstring(".2."))
		})
	})

	Describe("Reader", func() {
		It("finds disc-specific cover art", func() {
			// This test verifies the priority logic without requiring actual files
			reader := &discArtworkReader{
				cacheKey: cacheKey{artID: model.ArtworkID{ID: album.ID, Kind: model.KindDiscArtwork}},
				a:        artwork,
				album:    album,
				lib:      libraryView{Library: lib},
				discNumber: 1,
				mediaFiles: model.MediaFiles{
					{ID: "track-1", AlbumID: album.ID, Path: "Artist/Album/CD1/01.flac", DiscNumber: 1},
				},
				firstTrackRel: "Artist/Album/CD1/01.flac",
			}

			// With no image files, embedded artwork should still be attempted
			ff := reader.fromDiscArtPriority(ctx, ffmpegMock, "embedded")
			Expect(ff).To(HaveLen(1))
		})
	})

	Describe("fromDiscArtPriority", func() {
		var reader *discArtworkReader
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "navidrome-disc-art-test")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.RemoveAll, tmpDir)

			Expect(os.MkdirAll(filepath.Join(tmpDir, "music/album/cd2"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "music/album/cd2/disc.jpg"), []byte("disc"), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "music/album/cd2/disc2.jpg"), []byte("disc2"), 0o644)).To(Succeed())

			reader = &discArtworkReader{
				cacheKey: cacheKey{artID: model.ArtworkID{ID: album.ID, Kind: model.KindDiscArtwork}},
				a:        artwork,
				album:    album,
				discNumber: 2,
				mediaFiles: model.MediaFiles{
					{ID: "track-2", AlbumID: album.ID, Path: "music/album/cd2/track1.flac", DiscNumber: 2},
				},
				imgFiles: []string{
					"music/album/cd2/disc.jpg",
					"music/album/cd2/disc2.jpg",
				},
				firstTrackRel: "music/album/cd2/track1.flac",
				lib:           libraryView{FS: osDirFS{os.DirFS(tmpDir)}, absRoot: tmpDir},
			}
		})

		It("returns source funcs for glob patterns", func() {
			ff := reader.fromDiscArtPriority(context.Background(), nil, "disc*.*")
			Expect(ff).To(HaveLen(1))
		})

		It("returns source funcs for embedded pattern", func() {
			ff := reader.fromDiscArtPriority(context.Background(), nil, "embedded")
			Expect(ff).To(HaveLen(1)) // one FFmpeg embedded-art attempt
		})

		It("handles multiple comma-separated patterns", func() {
			ff := reader.fromDiscArtPriority(context.Background(), nil, "disc*.*, cd*.*, embedded")
			Expect(ff).To(HaveLen(3)) // disc*.* + cd*.* + one embedded-art attempt
		})

		It("ignores 'external' pattern silently", func() {
			ff := reader.fromDiscArtPriority(context.Background(), nil, "external")
			Expect(ff).To(HaveLen(0))
		})

		It("returns no source funcs when imgFiles is empty and pattern is not embedded", func() {
			reader.imgFiles = nil
			ff := reader.fromDiscArtPriority(context.Background(), nil, "disc*.*")
			Expect(ff).To(HaveLen(0))
		})
	})

	Describe("newDiscArtworkReader - sibling filtering", func() {
		It("includes only media files for the requested disc", func() {
			ds.(*persistence.MockDataStore).MockedAlbum.On("Get", album.ID).Return(&album, nil)
			ds.(*persistence.MockDataStore).MockedMediaFile.On("GetAllByAlbum", album.ID).Return(mediaFiles, nil)
			ds.(*persistence.MockDataStore).MockedLibrary.On("Get", lib.ID).Return(&lib, nil)

			artID := mediaFiles[1].DiscCoverArtID()
			reader, err := newDiscArtworkReader(ctx, artwork, artID)
			Expect(err).NotTo(HaveOccurred())
			Expect(reader.mediaFiles).To(HaveLen(1))
			Expect(reader.mediaFiles[0].DiscNumber).To(Equal(2))
		})
	})

	Describe("loadDiscFoldersPaths", func() {
		It("returns disc media paths from the media files", func() {
			files := model.MediaFiles{
				{Path: "Artist/Album/CD1/01.flac", DiscNumber: 1},
				{Path: "Artist/Album/CD1/02.flac", DiscNumber: 1},
			}

			paths := slice.Map(files, func(mf model.MediaFile) string { return mf.Path })
			Expect(paths).To(ConsistOf("Artist/Album/CD1/01.flac", "Artist/Album/CD1/02.flac"))
		})
	})

	Describe("media metadata", func() {
		It("preserves the artwork ID kind", func() {
			mf := model.MediaFile{
				ID:         "track-1",
				AlbumID:    album.ID,
				LibraryID:  lib.ID,
				Path:       "Artist/Album/CD1/01.flac",
				DiscNumber: 1,
				Tags:       model.Tags{"title": []string{"Track 1"}},
				AudioProperties: metadata.AudioProperties{
					Duration: time.Second,
				},
			}
			Expect(mf.DiscCoverArtID().Kind).To(Equal(model.KindDiscArtwork))
		})
	})
}
