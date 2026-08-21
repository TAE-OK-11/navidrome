//go:build !windows

package plugins

import (
	"errors"

	"github.com/navidrome/navidrome/core/agents"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("metadata agent error classification", func() {
	It("treats an unimplemented method as a definitive miss", func() {
		err := agentErr(errNotImplemented)
		Expect(errors.Is(err, agents.ErrNotFound)).To(BeTrue())
		Expect(errors.Is(err, errNotImplemented)).To(BeTrue())
	})

	It("treats a missing export as a definitive miss", func() {
		err := agentErr(errFunctionNotFound)
		Expect(errors.Is(err, agents.ErrNotFound)).To(BeTrue())
		Expect(errors.Is(err, errFunctionNotFound)).To(BeTrue())
	})

	It("keeps provider and transport failures as real faults", func() {
		providerErr := errors.New("provider returned 429")
		err := agentErr(providerErr)
		Expect(errors.Is(err, agents.ErrNotFound)).To(BeFalse())
		Expect(errors.Is(err, providerErr)).To(BeTrue())
	})
})
