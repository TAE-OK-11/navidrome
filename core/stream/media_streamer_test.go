package stream_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing/iotest"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/core/stream"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MediaStreamer", func() {
	var streamer stream.MediaStreamer
	var ds model.DataStore
	var testCache stream.TranscodingCache
	ffmpeg := tests.NewMockFFmpeg("fake data")
	ctx := log.NewContext(context.TODO())

	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
		cacheDir, _ := os.MkdirTemp("", "file_caches")
		conf.Server.CacheFolder = conf.NewDir(cacheDir)
		conf.Server.TranscodingCacheSize = "100MB"
		ds = &tests.MockDataStore{MockedTranscoding: &tests.MockTranscodingRepo{}}
		ds.MediaFile(ctx).(*tests.MockMediaFileRepo).SetData(model.MediaFiles{
			{ID: "123", Path: "tests/fixtures/test.mp3", Suffix: "mp3", BitRate: 128, Duration: 257.0},
		})
		testCache = stream.NewTranscodingCache()
		Eventually(func() bool { return testCache.Available(context.TODO()) }, 10*time.Second).Should(BeTrue())
		streamer = stream.NewMediaStreamer(ds, ffmpeg, testCache)
	})
	AfterEach(func() {
		_ = os.RemoveAll(conf.Server.CacheFolder.String())
	})

	Context("NewStream", func() {
		var mf *model.MediaFile
		BeforeEach(func() {
			var err error
			mf, err = ds.MediaFile(ctx).Get("123")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns a seekable stream if format is 'raw'", func() {
			s, err := streamer.NewStream(ctx, mf, stream.Request{Format: "raw"})
			Expect(err).ToNot(HaveOccurred())
			Expect(s.Seekable()).To(BeTrue())
		})
		It("returns a seekable stream if no format is specified (direct play)", func() {
			s, err := streamer.NewStream(ctx, mf, stream.Request{})
			Expect(err).ToNot(HaveOccurred())
			Expect(s.Seekable()).To(BeTrue())
		})
		It("serves byte ranges from direct-play files", func() {
			s, err := streamer.NewStream(ctx, mf, stream.Request{Format: "raw"})
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(s.Close)

			expected, err := os.ReadFile(mf.AbsolutePath())
			Expect(err).ToNot(HaveOccurred())
			req := httptest.NewRequest(http.MethodGet, "/stream", nil)
			req.Header.Set("Range", "bytes=1-4")
			resp := httptest.NewRecorder()

			_, err = s.Serve(ctx, resp, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.Code).To(Equal(http.StatusPartialContent))
			Expect(resp.Header().Get("Content-Range")).To(Equal("bytes 1-4/" + fmt.Sprint(len(expected))))
			Expect(resp.Body.Bytes()).To(Equal(expected[1:5]))
		})
		It("returns a NON seekable stream if transcode is required", func() {
			s, err := streamer.NewStream(ctx, mf, stream.Request{Format: "mp3", BitRate: 64})
			Expect(err).To(BeNil())
			Expect(s.Seekable()).To(BeFalse())
			Expect(s.Duration()).To(Equal(float32(257.0)))
		})
		It("routes raw direct play with transcodeOffset through the transcode pipeline", func() {
			s, err := streamer.NewStream(ctx, mf, stream.Request{Format: "raw", Offset: 30})
			Expect(err).ToNot(HaveOccurred())
			Expect(s.Seekable()).To(BeFalse())
		})
		It("does not consume a non-seekable transcode stream for HEAD requests", func() {
			reader := &readTrackingCloser{reads: make(chan struct{}, 1)}
			s := stream.NewStream(mf, "mp3", 64, reader)
			DeferCleanup(s.Close)
			req := httptest.NewRequest(http.MethodHead, "/stream", nil)
			resp := httptest.NewRecorder()

			n, err := s.Serve(ctx, resp, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(n).To(BeZero())
			Consistently(reader.reads, 100*time.Millisecond).ShouldNot(Receive())
		})
		It("treats a canceled client write as a normal stream close", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			writer := &cancelingResponseWriter{header: make(http.Header), cancel: cancel}
			s := stream.NewStream(mf, "mp3", 64, io.NopCloser(strings.NewReader("audio")))
			DeferCleanup(s.Close)
			req := httptest.NewRequest(http.MethodGet, "/stream", nil).WithContext(cancelCtx)

			n, err := s.Serve(cancelCtx, writer, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(n).To(BeZero())
		})
		It("rejects transcode requests beyond MaxConcurrent with ErrTooManyTranscodes", func() {
			// Use an ffmpeg whose Read blocks indefinitely so the cache's
			// background copy can't drain the source and release the slot —
			// keeping the single transcode slot pinned for this test.
			pr, pw := io.Pipe()
			DeferCleanup(func() { _ = pw.Close() })
			blockingFFmpeg := tests.NewMockFFmpeg("")
			blockingFFmpeg.Reader = pr

			conf.Server.Transcoding.MaxConcurrent = 1
			conf.Server.Transcoding.MaxConcurrentPerUser = 0
			tightCache := stream.NewTranscodingCache()
			Eventually(func() bool { return tightCache.Available(context.TODO()) }, 10*time.Second).Should(BeTrue())
			tightStreamer := stream.NewMediaStreamer(ds, blockingFFmpeg, tightCache)

			userCtx := request.WithUsername(ctx, "alice")
			s1, err := tightStreamer.NewStream(userCtx, mf, stream.Request{Format: "mp3", BitRate: 64})
			Expect(err).ToNot(HaveOccurred())
			defer s1.Close()

			// Different cache key so it doesn't dedupe with the first request.
			_, err = tightStreamer.NewStream(userCtx, mf, stream.Request{Format: "mp3", BitRate: 96})
			Expect(errors.Is(err, stream.ErrTooManyTranscodes)).To(BeTrue())
		})

		It("releases the slot once the stream is closed", func() {
			conf.Server.Transcoding.MaxConcurrent = 1
			conf.Server.Transcoding.MaxConcurrentPerUser = 0
			tightCache := stream.NewTranscodingCache()
			Eventually(func() bool { return tightCache.Available(context.TODO()) }, 10*time.Second).Should(BeTrue())
			tightStreamer := stream.NewMediaStreamer(ds, ffmpeg, tightCache)

			userCtx := request.WithUsername(ctx, "alice")
			s1, err := tightStreamer.NewStream(userCtx, mf, stream.Request{Format: "mp3", BitRate: 64})
			Expect(err).ToNot(HaveOccurred())
			_, _ = io.ReadAll(s1)
			_ = s1.Close()
			Eventually(func() bool { return ffmpeg.IsClosed() }, "3s").Should(BeTrue())

			// Slot should now be free for a different transcode.
			s2, err := tightStreamer.NewStream(userCtx, mf, stream.Request{Format: "mp3", BitRate: 96})
			Expect(err).ToNot(HaveOccurred())
			defer s2.Close()
		})

		It("does not consume a slot for raw streams", func() {
			conf.Server.Transcoding.MaxConcurrent = 1
			conf.Server.Transcoding.MaxConcurrentPerUser = 0
			tightCache := stream.NewTranscodingCache()
			Eventually(func() bool { return tightCache.Available(context.TODO()) }, 10*time.Second).Should(BeTrue())
			tightStreamer := stream.NewMediaStreamer(ds, ffmpeg, tightCache)

			userCtx := request.WithUsername(ctx, "alice")
			// First, saturate the single transcode slot.
			s1, err := tightStreamer.NewStream(userCtx, mf, stream.Request{Format: "mp3", BitRate: 64})
			Expect(err).ToNot(HaveOccurred())
			defer s1.Close()

			// Raw stream must still succeed.
			s2, err := tightStreamer.NewStream(userCtx, mf, stream.Request{Format: "raw"})
			Expect(err).ToNot(HaveOccurred())
			defer s2.Close()
		})

		It("returns a seekable stream if the file is complete in the cache", func() {
			s, err := streamer.NewStream(ctx, mf, stream.Request{Format: "mp3", BitRate: 32})
			Expect(err).To(BeNil())
			_, _ = io.ReadAll(s)
			_ = s.Close()
			Eventually(func() bool { return ffmpeg.IsClosed() }, "3s").Should(BeTrue())

			s, err = streamer.NewStream(ctx, mf, stream.Request{Format: "mp3", BitRate: 32})
			Expect(err).To(BeNil())
			Expect(s.Seekable()).To(BeTrue())
		})
	})

	Context("Serve", func() {
		var mf *model.MediaFile
		BeforeEach(func() {
			var err error
			mf, err = ds.MediaFile(ctx).Get("123")
			Expect(err).ToNot(HaveOccurred())
		})

		It("keeps empty output a non-error, so callers still reply 200 with an empty body", func() {
			s := stream.NewStream(mf, "mp3", 128, io.NopCloser(bytes.NewReader(nil)))
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			n, err := s.Serve(ctx, w, r)

			Expect(err).ToNot(HaveOccurred())
			Expect(n).To(BeZero())
			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("aborts the response when the source fails after sending data", func() {
			src := io.NopCloser(io.MultiReader(
				bytes.NewReader(bytes.Repeat([]byte("a"), 64*1024)),
				iotest.ErrReader(errors.New("transcoder died")),
			))
			server := httptest.NewServer(serveHandler(stream.NewStream(mf, "mp3", 128, src)))
			DeferCleanup(server.Close)

			resp, err := http.Get(server.URL)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			_, err = io.ReadAll(resp.Body)
			Expect(err).To(HaveOccurred())
		})
	})
})

// Serve runs behind the real server's Recoverer, which must let ErrAbortHandler through.
func serveHandler(s *stream.Stream) http.Handler {
	return middleware.Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = s.Serve(r.Context(), w, r)
	}))
}

type readTrackingCloser struct {
	reads chan struct{}
}

func (r *readTrackingCloser) Read([]byte) (int, error) {
	r.reads <- struct{}{}
	return 0, io.EOF
}

func (*readTrackingCloser) Close() error { return nil }

type cancelingResponseWriter struct {
	header http.Header
	cancel context.CancelFunc
}

func (w *cancelingResponseWriter) Header() http.Header { return w.header }
func (*cancelingResponseWriter) WriteHeader(int)       {}
func (w *cancelingResponseWriter) Write([]byte) (int, error) {
	w.cancel()
	return 0, context.Canceled
}
