package cmd

import (
	"os"
	"path/filepath"

	"github.com/navidrome/navidrome/conf"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("secureConfiguredHotCachePath", func() {
	var (
		enabled     bool
		path        conf.Dir
		cacheFolder conf.Dir
	)

	BeforeEach(func() {
		enabled = conf.Server.HotCache.Enabled
		path = conf.Server.HotCache.Path
		cacheFolder = conf.Server.CacheFolder
	})

	AfterEach(func() {
		conf.Server.HotCache.Enabled = enabled
		conf.Server.HotCache.Path = path
		conf.Server.CacheFolder = cacheFolder
	})

	It("disables only Hot Cache when the configured directory is unrelated", func() {
		directory := GinkgoT().TempDir()
		importantPath := filepath.Join(directory, "important.flac")
		Expect(os.WriteFile(importantPath, []byte("keep"), 0o600)).To(Succeed())
		conf.Server.HotCache.Enabled = true
		conf.Server.HotCache.Path = conf.NewDir(directory)

		secureConfiguredHotCachePath()

		Expect(conf.Server.HotCache.Enabled).To(BeFalse())
		contents, err := os.ReadFile(importantPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(contents).To(Equal([]byte("keep")))
	})

	It("keeps Hot Cache enabled for a dedicated empty directory", func() {
		directory := filepath.Join(GinkgoT().TempDir(), "hot-cache")
		conf.Server.HotCache.Enabled = true
		conf.Server.HotCache.Path = conf.NewDir(directory)

		secureConfiguredHotCachePath()

		Expect(conf.Server.HotCache.Enabled).To(BeTrue())
		resolved, err := filepath.EvalSymlinks(directory)
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.Server.HotCache.Path.String()).To(Equal(resolved))
	})
})
