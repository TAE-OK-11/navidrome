package subsonic

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
)

var _ = Describe("API key auth params", func() {
	It("rejects apiKey combined with username", func() {
		r := newGetRequest("apiKey=abc", "u=user", "v=1.16.1", "c=test")
		_, p := req.WithParams(r)
		err := validateAPIKeyAuthParams(p)
		Expect(err).To(HaveOccurred())
		var subErr subError
		Expect(errors.As(err, &subErr)).To(BeTrue())
		Expect(subErr.code).To(Equal(responses.ErrorConflictingAuth))
	})

	It("rejects apiKey combined with password auth", func() {
		r := newGetRequest("apiKey=abc", "p=secret", "v=1.16.1", "c=test")
		_, p := req.WithParams(r)
		err := validateAPIKeyAuthParams(p)
		Expect(err).To(HaveOccurred())
		var subErr subError
		Expect(errors.As(err, &subErr)).To(BeTrue())
		Expect(subErr.code).To(Equal(responses.ErrorConflictingAuth))
	})

	It("accepts apiKey alone", func() {
		r := newGetRequest("apiKey=abc", "v=1.16.1", "c=test")
		_, p := req.WithParams(r)
		Expect(validateAPIKeyAuthParams(p)).To(Succeed())
	})
})
