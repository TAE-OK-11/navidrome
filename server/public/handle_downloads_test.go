package public

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type downloadMockArchiver struct {
	called bool
	err    error
}

func (m *downloadMockArchiver) ZipAlbum(context.Context, string, string, int, io.Writer) error {
	return nil
}

func (m *downloadMockArchiver) ZipArtist(context.Context, string, string, int, io.Writer) error {
	return nil
}

func (m *downloadMockArchiver) ZipPlaylist(context.Context, string, string, int, io.Writer) error {
	return nil
}

func (m *downloadMockArchiver) ZipShare(_ context.Context, _ string, w io.Writer) error {
	m.called = true
	if m.err != nil {
		return m.err
	}
	_, _ = w.Write([]byte("zip-contents"))
	return nil
}

var _ = Describe("handleDownloads", func() {
	var ds *tests.MockDataStore
	var shareRepo *tests.MockShareRepo
	var archiver *downloadMockArchiver
	var pub *Router

	BeforeEach(func() {
		ds = &tests.MockDataStore{}
		shareRepo = &tests.MockShareRepo{}
		ds.MockedShare = shareRepo
		archiver = &downloadMockArchiver{}
		pub = &Router{ds: ds, archiver: archiver, share: core.NewShare(ds)}
	})

	shareIs := func(s *model.Share) {
		shareRepo.ID = s.ID
		shareRepo.Entity = s
	}

	makeRequest := func(id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/public/d/"+id+"?%3Aid="+id, nil)
		w := httptest.NewRecorder()
		pub.handleDownloads(w, r)
		return w
	}

	It("sets a safe filename and content type before streaming", func() {
		shareIs(&model.Share{ID: "abc123", Description: "My Mixtape", Downloadable: true})

		w := makeRequest("abc123")

		Expect(w.Code).To(Equal(http.StatusOK))
		mediaType, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
		Expect(err).ToNot(HaveOccurred())
		Expect(mediaType).To(Equal("attachment"))
		Expect(params["filename"]).To(Equal("My Mixtape.zip"))
		Expect(w.Header().Get("Content-Type")).To(Equal("application/zip"))
		Expect(archiver.called).To(BeTrue())
		Expect(w.Body.String()).To(Equal("zip-contents"))
	})

	It("encodes a Unicode filename without corrupting it", func() {
		shareIs(&model.Share{ID: "abc123", Description: "태영 음악", Downloadable: true})

		w := makeRequest("abc123")

		_, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
		Expect(err).ToNot(HaveOccurred())
		Expect(params["filename"]).To(Equal("태영 음악.zip"))
	})

	It("falls back to the share ID when there is no description", func() {
		shareIs(&model.Share{ID: "abc123", Downloadable: true})

		w := makeRequest("abc123")

		_, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
		Expect(err).ToNot(HaveOccurred())
		Expect(params["filename"]).To(Equal("abc123.zip"))
	})

	It("returns 403 without invoking the archiver when downloading is disabled", func() {
		shareIs(&model.Share{ID: "abc123", Description: "No Download", Downloadable: false})

		w := makeRequest("abc123")

		Expect(w.Code).To(Equal(http.StatusForbidden))
		Expect(archiver.called).To(BeFalse())
		Expect(w.Header().Get("Content-Disposition")).To(BeEmpty())
	})

	It("returns 404 when the share does not exist", func() {
		shareIs(&model.Share{ID: "other", Downloadable: true})

		w := makeRequest("missing")

		Expect(w.Code).To(Equal(http.StatusNotFound))
		Expect(archiver.called).To(BeFalse())
	})

	It("returns 410 when the share has expired", func() {
		shareIs(&model.Share{ID: "abc123", Downloadable: true, ExpiresAt: new(time.Now().Add(-time.Hour))})

		w := makeRequest("abc123")

		Expect(w.Code).To(Equal(http.StatusGone))
		Expect(archiver.called).To(BeFalse())
	})

	It("returns 500 when the share lookup fails", func() {
		shareRepo.Error = errors.New("db error")

		w := makeRequest("abc123")

		Expect(w.Code).To(Equal(http.StatusInternalServerError))
		Expect(archiver.called).To(BeFalse())
	})
})
