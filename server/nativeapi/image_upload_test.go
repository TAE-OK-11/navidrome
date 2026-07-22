package nativeapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func syntheticUploadPNGHeader(width, height uint32) []byte {
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	_ = binary.Write(&out, binary.BigEndian, uint32(13))
	chunk := make([]byte, 17)
	copy(chunk, "IHDR")
	binary.BigEndian.PutUint32(chunk[4:8], width)
	binary.BigEndian.PutUint32(chunk[8:12], height)
	chunk[12] = 8
	chunk[13] = 2
	out.Write(chunk)
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return out.Bytes()
}

var _ = Describe("maxImageUploadSize", func() {
	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
	})

	It("returns the configured size when valid", func() {
		conf.Server.MaxImageUploadSize = "20MB"
		Expect(maxImageUploadSize()).To(Equal(int64(20_000_000)))
	})

	It("returns the default size when config is empty", func() {
		conf.Server.MaxImageUploadSize = ""
		Expect(maxImageUploadSize()).To(Equal(int64(10_000_000)))
	})

	It("returns the default size when config is invalid", func() {
		conf.Server.MaxImageUploadSize = "not-a-size"
		Expect(maxImageUploadSize()).To(Equal(int64(10_000_000)))
	})

	It("parses raw byte values", func() {
		conf.Server.MaxImageUploadSize = "52428800"
		Expect(maxImageUploadSize()).To(Equal(int64(52_428_800)))
	})
})

var _ = Describe("image upload dimension limits", func() {
	BeforeEach(func() {
		DeferCleanup(configtest.SetupConfig())
		conf.Server.EnableArtworkUpload = true
	})

	It("rejects excessive DecodeConfig dimensions before save", func() {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("image", "huge.png")
		Expect(err).ToNot(HaveOccurred())
		_, err = part.Write(syntheticUploadPNGHeader(10_000, 5_000))
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.Close()).To(Succeed())

		req := httptest.NewRequest(http.MethodPost, "/image", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req = req.WithContext(request.WithUser(req.Context(), model.User{ID: "ordinary-user"}))
		resp := httptest.NewRecorder()
		saved := false
		handleImageUpload(func(context.Context, io.Reader, string) error {
			saved = true
			return nil
		})(resp, req)

		Expect(resp.Code).To(Equal(http.StatusBadRequest))
		Expect(saved).To(BeFalse())
	})

	It("continues to accept a normal small image", func() {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("image", "small.png")
		Expect(err).ToNot(HaveOccurred())
		_, err = part.Write(syntheticUploadPNGHeader(32, 32))
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.Close()).To(Succeed())

		req := httptest.NewRequest(http.MethodPost, "/image", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req = req.WithContext(request.WithUser(req.Context(), model.User{ID: "ordinary-user"}))
		resp := httptest.NewRecorder()
		saved := false
		handleImageUpload(func(_ context.Context, reader io.Reader, ext string) error {
			saved = true
			Expect(ext).To(Equal(".png"))
			data, readErr := io.ReadAll(reader)
			Expect(readErr).ToNot(HaveOccurred())
			Expect(data).ToNot(BeEmpty())
			return nil
		})(resp, req)

		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(saved).To(BeTrue())
	})
})
