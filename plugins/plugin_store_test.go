//go:build !windows

package plugins

import (
	"database/sql"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/persistence"
	. "github.com/onsi/gomega"
)

func newTestPluginStore(dir string) model.DataStore {
	path := filepath.Join(dir, "navidrome.db")
	conn, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_foreign_keys=on")
	Expect(err).ToNot(HaveOccurred())
	Expect(persistence.EnsurePluginRuntimeSchema(conn)).To(Succeed())
	return persistence.New(conn)
}
