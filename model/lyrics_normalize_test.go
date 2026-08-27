package model

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func p(v int64) *int64 { return &v }

var _ = Describe("NormalizeCueEnds", func() {
	It("returns nil/empty inputs unchanged", func() {
		Expect(NormalizeCueEnds(nil, p(1000))).To(BeNil())
		Expect(NormalizeCueEnds([]Cue{}, p(1000))).To(BeEmpty())
	})

	It("fills missing ends from the next cue start", func() {
		cues := []Cue{
			{Start: p(1000), Value: "a"},
			{Start: p(2000), Value: "b"},
		}
		out := NormalizeCueEnds(cues, p(3000))
		Expect(out[0].End).To(Equal(p(2000)))
		Expect(out[1].End).To(Equal(p(3000)))
	})

	It("clamps ends that overrun the next cue", func() {
		cues := []Cue{
			{Start: p(1000), End: p(2500), Value: "a"},
			{Start: p(2000), Value: "b"},
		}
		out := NormalizeCueEnds(cues, p(3000))
		Expect(out[0].End).To(Equal(p(2000)))
	})

	It("clears all ends when any cue still lacks one", func() {
		cues := []Cue{
			{Start: p(1000), Value: "a"},
			{Start: p(2000), Value: "b"},
		}
		out := NormalizeCueEnds(cues, nil)
		for _, cue := range out {
			Expect(cue.End).To(BeNil())
		}
	})

	It("does not mutate the input slice", func() {
		cues := []Cue{{Start: p(1000), Value: "a"}, {Start: p(2000), Value: "b"}}
		_ = NormalizeCueEnds(cues, p(3000))
		Expect(cues[0].End).To(BeNil())
	})
})
