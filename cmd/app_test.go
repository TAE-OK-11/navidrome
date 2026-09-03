package cmd

import (
	"github.com/navidrome/navidrome/plugins"
	"github.com/navidrome/navidrome/server/subsonic"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("App composition root", func() {
	It("keeps HTTP, Subsonic, and plugin manager on the same object graph", func() {
		sub := &subsonic.Router{}
		mgr := &plugins.Manager{}
		ds := &tests.MockDataStore{}

		app := newApp(nil, nil, sub, nil, nil, nil, nil, nil, nil, nil, nil, nil, mgr, ds)

		Expect(app.SubsonicAPI).To(BeIdenticalTo(sub))
		Expect(app.Plugins).To(BeIdenticalTo(mgr))
		Expect(app.DataStore).To(BeIdenticalTo(ds))
	})
})
