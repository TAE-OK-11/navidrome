package librefm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLibreFM(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Libre.fm Test Suite")
}
