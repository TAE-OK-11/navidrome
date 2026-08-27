package metadata_test

import (
	"os"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/metadata"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ToMediaFile", func() {
	var (
		filePath string
		props    metadata.Info
		mf       model.MediaFile
	)

	BeforeEach(func() {
		_, filePath, _ = tests.TempFile(GinkgoT(), "test", ".mp3")
		fileInfo, _ := os.Stat(filePath)
		props = metadata.Info{
			FileInfo: testFileInfo{fileInfo},
		}
	})

	var toMediaFile = func(tags model.RawTags) model.MediaFile {
		if _, ok := tags["TITLE"]; !ok {
			tags["TITLE"] = []string{"Test Track"}
		}
		if _, ok := tags["ALBUM"]; !ok {
			tags["ALBUM"] = []string{"Test Album"}
		}
		return toMediaFileFromTags(GinkgoT(), filePath, props, tags)
	}

	Describe("Dates", func() {
		It("should parse properly tagged dates ", func() {
			mf = toMediaFile(model.RawTags{
				"ORIGINALDATE": {"1978-09-10"},
				"DATE":         {"1977-03-04"},
				"RELEASEDATE":  {"2002-01-02"},
			})

			Expect(mf.Year).To(Equal(1977))
			Expect(mf.Date).To(Equal("1977-03-04"))
			Expect(mf.OriginalYear).To(Equal(1978))
			Expect(mf.OriginalDate).To(Equal("1978-09-10"))
			Expect(mf.ReleaseYear).To(Equal(2002))
			Expect(mf.ReleaseDate).To(Equal("2002-01-02"))
		})

		It("should parse dates with only year", func() {
			mf = toMediaFile(model.RawTags{
				"ORIGINALYEAR": {"1978"},
				"DATE":         {"1977"},
				"RELEASEDATE":  {"2002"},
			})

			Expect(mf.Year).To(Equal(1977))
			Expect(mf.Date).To(Equal("1977"))
			Expect(mf.OriginalYear).To(Equal(1978))
			Expect(mf.OriginalDate).To(Equal("1978"))
			Expect(mf.ReleaseYear).To(Equal(2002))
			Expect(mf.ReleaseDate).To(Equal("2002"))
		})

		It("should parse dates tagged the legacy way (no release date)", func() {
			mf = toMediaFile(model.RawTags{
				"DATE":         {"2014"},
				"ORIGINALDATE": {"1966"},
			})

			Expect(mf.Year).To(Equal(1966))
			Expect(mf.OriginalYear).To(Equal(1966))
			Expect(mf.ReleaseYear).To(Equal(2014))
		})
		DescribeTable("legacyReleaseDate (TaggedLikePicard old behavior)",
			func(recordingDate, originalDate, releaseDate, expected string) {
				mf := toMediaFile(model.RawTags{
					"DATE":         {recordingDate},
					"ORIGINALDATE": {originalDate},
					"RELEASEDATE":  {releaseDate},
				})

				Expect(mf.ReleaseDate).To(Equal(expected))
			},
			Entry("regular mapping", "2020-05-15", "2019-02-10", "2021-01-01", "2021-01-01"),
			Entry("legacy mapping", "2020-05-15", "2019-02-10", "", "2020-05-15"),
			Entry("legacy mapping, originalYear < year", "2018-05-15", "2019-02-10", "2021-01-01", "2021-01-01"),
			Entry("legacy mapping, originalYear empty", "2020-05-15", "", "2021-01-01", "2021-01-01"),
			Entry("legacy mapping, releaseYear", "2020-05-15", "2019-02-10", "2021-01-01", "2021-01-01"),
			Entry("legacy mapping, same dates", "2020-05-15", "2020-05-15", "", "2020-05-15"),
		)
	})

	Describe("Lyrics", func() {
		It("uses Rust pre-parsed lyrics_json when present", func() {
			props.Tags = model.RawTags{
				"TITLE":      {"Song"},
				"ALBUM":      {"Album"},
				"LYRICS:ENG": {"ignored by Rust path"},
			}
			props.LyricsJSON = `[{"lang":"eng","line":[{"value":"from rust","start":1000}],"synced":true}]`
			props = propsWithRustMediaFile(GinkgoT(), "song.mp3", props)
			Expect(metadata.New("song.mp3", props).ToMediaFile(1, "folderID").Lyrics).To(Equal(
				`[{"lang":"eng","line":[{"value":"from rust","start":1000}],"synced":true}]`,
			))
		})
	})

	Describe("BPM", func() {
		It("maps the BPM tag rounded to the nearest integer", func() {
			mf = toMediaFile(model.RawTags{"BPM": {"120.6"}})
			Expect(mf.BPM).To(Equal(new(121)))
		})
		It("leaves BPM nil when the tag is absent", func() {
			mf = toMediaFile(model.RawTags{})
			Expect(mf.BPM).To(BeNil())
		})
		It("leaves BPM nil when the tag is zero or unparseable", func() {
			Expect(toMediaFile(model.RawTags{"BPM": {"0"}}).BPM).To(BeNil())
			Expect(toMediaFile(model.RawTags{"BPM": {"fast"}}).BPM).To(BeNil())
		})
	})

	Describe("BitDepth", func() {
		It("maps the bit depth when present", func() {
			props.AudioProperties = metadata.AudioProperties{BitDepth: 24}
			mf = toMediaFile(model.RawTags{})
			Expect(mf.BitDepth).To(Equal(new(24)))
		})
		It("leaves BitDepth nil when zero (lossy codecs have no bit depth)", func() {
			props.AudioProperties = metadata.AudioProperties{BitDepth: 0}
			mf = toMediaFile(model.RawTags{})
			Expect(mf.BitDepth).To(BeNil())
		})
	})
})
