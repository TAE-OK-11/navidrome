package subsonic

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subsonic version negotiation", func() {
	DescribeTable("compareSubsonicVersion",
		func(a, b string, expected int) {
			cmp, err := compareSubsonicVersion(a, b)
			Expect(err).ToNot(HaveOccurred())
			Expect(cmp).To(Equal(expected))
		},
		Entry("equal versions", "1.16.1", "1.16.1", 0),
		Entry("client newer patch", "1.16.2", "1.16.1", 1),
		Entry("client older patch", "1.16.0", "1.16.1", -1),
		Entry("client newer minor", "1.17.0", "1.16.1", 1),
		Entry("client older minor", "1.15.0", "1.16.1", -1),
		Entry("two-part version", "1.8", "1.8.0", 0),
		Entry("minimum supported", "1.8.0", MinSupportedVersion, 0),
	)

	It("rejects malformed versions", func() {
		_, err := compareSubsonicVersion("bad", "1.16.1")
		Expect(err).To(HaveOccurred())
	})

	DescribeTable("validateClientVersion",
		func(clientVersion string, expectErr bool, code int32) {
			err := validateClientVersion(clientVersion)
			if !expectErr {
				Expect(err).ToNot(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			var subErr subError
			Expect(errors.As(err, &subErr)).To(BeTrue())
			Expect(subErr.code).To(Equal(code))
		},
		Entry("supported version", "1.16.1", false, int32(0)),
		Entry("minimum supported version", "1.8.0", false, int32(0)),
		Entry("too old client", "1.7.0", true, int32(20)),
		Entry("newer than server", "9.9.9", true, int32(30)),
		Entry("invalid version is ignored", "not-a-version", false, int32(0)),
	)

	It("validateClientVersion returns subError codes", func() {
		err := validateClientVersion("1.7.0")
		Expect(err).To(HaveOccurred())
		var subErr subError
		Expect(errors.As(err, &subErr)).To(BeTrue())
		Expect(subErr.code).To(Equal(int32(20)))
	})
})
