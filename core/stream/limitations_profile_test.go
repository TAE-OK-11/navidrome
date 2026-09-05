package stream

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("applyLimitation audio profile", func() {
	It("rejects when profile does not match required equals limitation", func() {
		ts := &Details{Profile: "HE-AAC"}
		result := applyLimitation(256, &Limitation{
			Name:       LimitationAudioProfile,
			Comparison: ComparisonEquals,
			Values:     []string{"LC"},
		}, ts)
		Expect(result).To(Equal(adjustCannotFit))
	})

	It("passes when profile matches equals limitation", func() {
		ts := &Details{Profile: "LC"}
		result := applyLimitation(256, &Limitation{
			Name:       LimitationAudioProfile,
			Comparison: ComparisonEquals,
			Values:     []string{"LC"},
		}, ts)
		Expect(result).To(Equal(adjustNone))
	})

	It("fails required profile checks when profile is missing", func() {
		ts := &Details{}
		result := applyLimitation(256, &Limitation{
			Name:       LimitationAudioProfile,
			Comparison: ComparisonEquals,
			Values:     []string{"LC"},
		}, ts)
		Expect(result).To(Equal(adjustCannotFit))
	})
})
