// nolint unused
package scanner

import (
	"context"
	"errors"
	"sync/atomic"

	ppl "github.com/google/go-pipeline/pkg/pipeline"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type mockPhase struct {
	num           int
	produceFunc   func() ppl.Producer[int]
	stagesFunc    func() []ppl.Stage[int]
	finalizeFunc  func(error) error
	descriptionFn func() string
}

func (m *mockPhase) producer() ppl.Producer[int] {
	return m.produceFunc()
}

func (m *mockPhase) stages() []ppl.Stage[int] {
	return m.stagesFunc()
}

func (m *mockPhase) finalize(err error) error {
	return m.finalizeFunc(err)
}

func (m *mockPhase) description() string {
	return m.descriptionFn()
}

var _ = Describe("runPhase", func() {
	var (
		ctx      context.Context
		phaseNum int
		phase    *mockPhase
		sum      atomic.Int32
	)

	BeforeEach(func() {
		ctx = context.Background()
		phaseNum = 1
		phase = &mockPhase{
			num: 3,
			produceFunc: func() ppl.Producer[int] {
				return ppl.NewProducer(func(put func(int)) error {
					for i := 1; i <= phase.num; i++ {
						put(i)
					}
					return nil
				})
			},
			stagesFunc: func() []ppl.Stage[int] {
				return []ppl.Stage[int]{ppl.NewStage(func(i int) (int, error) {
					sum.Add(int32(i))
					return i, nil
				})}
			},
			finalizeFunc: func(err error) error {
				return err
			},
			descriptionFn: func() string {
				return "Mock Phase"
			},
		}
	})

	It("should run the phase successfully", func() {
		err := runPhase(ctx, phaseNum, phase)()
		Expect(err).ToNot(HaveOccurred())
		Expect(sum.Load()).To(Equal(int32(1 * 2 * 3)))
	})

	It("should log an error if the phase fails", func() {
		phase.finalizeFunc = func(err error) error {
			return errors.New("finalize error")
		}
		err := runPhase(ctx, phaseNum, phase)()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("finalize error"))
	})

	It("should count the tasks", func() {
		counter, countStageFn := countTasks[int]()
		phase.stagesFunc = func() []ppl.Stage[int] {
			return []ppl.Stage[int]{ppl.NewStage(countStageFn, ppl.Name("count tasks"))}
		}
		err := runPhase(ctx, phaseNum, phase)()
		Expect(err).ToNot(HaveOccurred())
		Expect(counter.Load()).To(Equal(int64(3)))
	})
})

var _ = Describe("audio probe fallback helpers", func() {
	Describe("mergeAudioProbeProperties", func() {
		It("fills only missing audio properties", func() {
			track := &model.MediaFile{Codec: "AAC"}
			probe := &ffmpeg.AudioProbeResult{
				Codec:      "aac",
				BitRate:    256,
				SampleRate: 48000,
				BitDepth:   16,
				Channels:   2,
			}

			mergeAudioProbeProperties(track, probe)

			Expect(track.Codec).To(Equal("AAC"))
			Expect(track.BitRate).To(Equal(256))
			Expect(track.SampleRate).To(Equal(48000))
			Expect(track.Channels).To(Equal(2))
			Expect(*track.BitDepth).To(Equal(16))
		})

		It("does not overwrite values already read from tags", func() {
			bitDepth := 24
			track := &model.MediaFile{
				Codec:      "ALAC",
				BitRate:    900,
				SampleRate: 44100,
				BitDepth:   &bitDepth,
				Channels:   2,
			}
			probe := &ffmpeg.AudioProbeResult{
				Codec:      "aac",
				BitRate:    256,
				SampleRate: 48000,
				BitDepth:   16,
				Channels:   6,
			}

			mergeAudioProbeProperties(track, probe)

			Expect(track.Codec).To(Equal("ALAC"))
			Expect(track.BitRate).To(Equal(900))
			Expect(track.SampleRate).To(Equal(44100))
			Expect(track.Channels).To(Equal(2))
			Expect(*track.BitDepth).To(Equal(24))
		})
	})

	Describe("scannerProbePath", func() {
		It("accepts local filesystem paths", func() {
			probePath, ok := scannerProbePath("/music", "Album/track.m4a")
			Expect(ok).To(BeTrue())
			Expect(probePath).To(Equal("/music/Album/track.m4a"))
		})

		It("accepts file URLs", func() {
			probePath, ok := scannerProbePath("file:///music", "Album/track.m4a")
			Expect(ok).To(BeTrue())
			Expect(probePath).To(Equal("/music/Album/track.m4a"))
		})

		It("skips non-local storage", func() {
			_, ok := scannerProbePath("s3://bucket/music", "Album/track.m4a")
			Expect(ok).To(BeFalse())
		})
	})
})
