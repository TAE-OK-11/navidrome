package artwork

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/metadata"
	"github.com/navidrome/navidrome/tests"
)

func syntheticPNGHeader(width, height uint32) []byte {
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

func TestSafeArtworkDialerBlocksResolvedPrivateAddressAndRedirect(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		_, _ = w.Write([]byte("private"))
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://redirected.test/secret", http.StatusFound)
	}))
	defer redirect.Close()

	dialer := safeArtworkDialer{
		lookup: func(_ context.Context, _, host string) ([]net.IP, error) {
			switch host {
			case "initial.test":
				return []net.IP{net.ParseIP("8.8.8.8")}, nil
			case "redirected.test":
				return []net.IP{net.ParseIP("127.0.0.1")}, nil
			default:
				return nil, errors.New("unexpected host")
			}
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			if strings.HasPrefix(address, "8.8.8.8:") {
				return d.DialContext(ctx, network, redirect.Listener.Addr().String())
			}
			return d.DialContext(ctx, network, target.Listener.Addr().String())
		},
	}
	client := &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}
	_, err := client.Get("http://initial.test/start")
	if err == nil || !strings.Contains(err.Error(), "disallowed address") {
		t.Fatalf("expected redirected private address to be blocked, got %v", err)
	}
	if targetHits.Load() != 0 {
		t.Fatal("private redirect target was reached")
	}
}

func TestArtworkDestinationIPScope(t *testing.T) {
	for _, raw := range []string{
		"0.0.0.0", "127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254",
		"192.0.2.1", "198.18.0.1", "224.0.0.1", "240.0.0.1", "::1", "fc00::1", "fe80::1", "2001:db8::1",
	} {
		if isSafeArtworkIP(net.ParseIP(raw)) {
			t.Errorf("expected special-use address %s to be rejected", raw)
		}
	}
	if !isSafeArtworkIP(net.ParseIP("8.8.8.8")) || !isSafeArtworkIP(net.ParseIP("2606:4700:4700::1111")) {
		t.Fatal("expected representative public unicast addresses to remain allowed")
	}
}

func TestFromURLBlocksInitialLoopback(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("secret"))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	_, _, err := fromURL(context.Background(), u)
	if err == nil || hits.Load() != 0 {
		t.Fatalf("expected initial loopback destination to be blocked before connection, err=%v hits=%d", err, hits.Load())
	}
}

func TestExternalArtworkBodyIsBounded(t *testing.T) {
	r := &boundedArtworkReadCloser{reader: io.NopCloser(strings.NewReader("abcd")), remaining: 3}
	data, err := io.ReadAll(r)
	if !errors.Is(err, errExternalArtworkTooLarge) {
		t.Fatalf("expected size-limit error, got %v", err)
	}
	if string(data) != "abc" {
		t.Fatalf("expected only bounded bytes, got %q", data)
	}
}

type resolvingTestFS struct {
	fs.FS
	root string
}

func (f resolvingTestFS) ReadTags(...string) (map[string]metadata.Info, error) { return nil, nil }
func (f resolvingTestFS) ResolveSymlink(name string) (string, error) {
	return filepath.EvalSymlinks(filepath.Join(f.root, filepath.FromSlash(name)))
}

func TestLibraryArtworkSymlinkContainment(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.jpg"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside.jpg"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.jpg"), filepath.Join(root, "outside.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "inside.jpg"), filepath.Join(root, "inside-link.jpg")); err != nil {
		t.Fatal(err)
	}
	view := libraryView{FS: resolvingTestFS{FS: os.DirFS(root), root: root}, absRoot: root}
	if _, err := view.OpenArtwork("outside.jpg"); err == nil {
		t.Fatal("expected outside-root symlink to be rejected")
	}
	r, err := view.OpenArtwork("inside-link.jpg")
	if err != nil {
		t.Fatalf("expected in-root symlink to remain supported: %v", err)
	}
	defer r.Close()
	data, _ := io.ReadAll(r)
	if string(data) != "inside" {
		t.Fatalf("unexpected in-root content %q", data)
	}
}

func TestPlaylistLocalArtworkSymlinkContainment(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	insidePath := filepath.Join(root, "inside.jpg")
	outsidePath := filepath.Join(outside, "secret")
	_ = os.WriteFile(insidePath, []byte("inside"), 0600)
	_ = os.WriteFile(outsidePath, []byte("outside"), 0600)
	insideLink := filepath.Join(root, "inside-link.jpg")
	outsideLink := filepath.Join(root, "outside-link.jpg")
	_ = os.Symlink(insidePath, insideLink)
	_ = os.Symlink(outsidePath, outsideLink)
	repo := &tests.MockLibraryRepo{}
	repo.SetData(model.Libraries{{ID: 1, Path: root}})
	ds := &tests.MockDataStore{MockedLibrary: repo}
	if _, _, err := fromLibraryLocalFile(context.Background(), ds, outsideLink)(); err == nil {
		t.Fatal("expected local playlist symlink escaping the library to be rejected")
	}
	r, _, err := fromLibraryLocalFile(context.Background(), ds, insideLink)()
	if err != nil {
		t.Fatalf("expected local playlist symlink staying in the library to be allowed: %v", err)
	}
	defer r.Close()
}

func TestLargeSyntheticDimensionsRejectedBeforeFullDecode(t *testing.T) {
	header := syntheticPNGHeader(50_000, 50_000)
	config, _, err := image.DecodeConfig(bytes.NewReader(header))
	if err != nil {
		t.Fatalf("DecodeConfig should accept the small synthetic header: %v", err)
	}
	if err := ValidateImageConfig(config); err == nil {
		t.Fatal("expected excessive dimensions to be rejected")
	}
	if _, _, err := DecodeImage(bytes.NewReader(header)); err == nil {
		t.Fatal("expected stream decoder to reject excessive dimensions before full decode")
	}
	if _, _, err := resizeStaticImage(header, 300, false); err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("expected resize preflight rejection before full decode, got %v", err)
	}
	if err := ValidateImageConfig(image.Config{Width: 10_000, Height: 5_000}); err == nil {
		t.Fatal("expected pixel budget to reject dimensions below the per-axis limit")
	}
	if err := ValidateImageConfig(image.Config{Width: 4_000, Height: 4_000}); err != nil {
		t.Fatalf("expected normal image dimensions to remain allowed: %v", err)
	}
}
