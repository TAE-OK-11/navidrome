package server

import (
	"context"
	"errors"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("initial_setup", func() {
	var ds model.DataStore

	BeforeEach(func() {
		ds = &tests.MockDataStore{}
	})

	Describe("createInitialAdminUser", func() {
		It("creates a new admin user with specified password if User table is empty", func() {
			Expect(createInitialAdminUser(ds, "pass123")).To(BeNil())
			ur := ds.User(context.TODO())
			admin, err := ur.FindByUsername("admin")
			Expect(err).To(BeNil())
			Expect(admin.Password).To(Equal("pass123"))
		})

		It("does not create a new admin user if User table is not empty", func() {
			Expect(createInitialAdminUser(ds, "first")).To(BeNil())
			ur := ds.User(context.TODO())
			Expect(ur.CountAll()).To(Equal(int64(1)))
			Expect(createInitialAdminUser(ds, "second")).To(BeNil())
			Expect(ur.CountAll()).To(Equal(int64(1)))
		})

		It("returns an error instead of panicking when the user repository fails", func() {
			repo := tests.CreateMockUserRepo()
			repo.Error = errors.New("user table unavailable")
			ds = &tests.MockDataStore{MockedUser: repo}

			Expect(createInitialAdminUser(ds, "pass123")).To(MatchError("user table unavailable"))
		})
	})

	Describe("initialSetup", func() {
		It("returns setup errors instead of ignoring them", func() {
			ds = &tests.MockDataStore{MockedProperty: &tests.MockedPropertyRepo{Error: errors.New("property store unavailable")}}

			Expect(initialSetup(ds)).To(MatchError("property store unavailable"))
		})
	})
})
