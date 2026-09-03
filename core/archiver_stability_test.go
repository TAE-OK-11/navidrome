package core_test

import (
	"bytes"
	"context"
	"errors"

	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/core/stream"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/query"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Archiver stability", func() {
	It("propagates ordinary stream creation failures instead of emitting a partial success", func() {
		streamErr := errors.New("transcoder failed")
		mfs := model.MediaFiles{
			{Path: "test_data/01 - track1.mp3", Suffix: "mp3", AlbumID: "1", Album: "Album", DiscNumber: 1},
			{Path: "test_data/02 - track2.mp3", Suffix: "mp3", AlbumID: "1", Album: "Album", DiscNumber: 1},
		}

		mfRepo := &mockMediaFileRepository{}
		mfRepo.On("GetAll", []model.QueryOptions{{
			Filters: query.Eq("album_id", "1"),
			Sort:    "album",
		}}).Return(mfs, nil)

		ds := &mockDataStore{}
		ds.On("MediaFile", mock.Anything).Return(mfRepo)
		ms := &mockMediaStreamer{}
		ms.On("NewStream", mock.Anything, mock.Anything, stream.Request{Format: "mp3", BitRate: 128}).
			Return(nil, streamErr).Once()

		arch := core.NewArchiver(ms, ds, &mockShare{})
		err := arch.ZipAlbum(context.Background(), "1", "mp3", 128, new(bytes.Buffer))

		Expect(err).To(MatchError(streamErr))
		ms.AssertNumberOfCalls(GinkgoT(), "NewStream", 1)
	})
})
