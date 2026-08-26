package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/log"
)

var once sync.Once

func Init(t *testing.T, skipOnShort bool) {
	if skipOnShort && testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	once.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		appPath, _ := filepath.Abs(filepath.Join(filepath.Dir(file), ".."))
		confPath, _ := filepath.Abs(filepath.Join(appPath, "tests", "navidrome-test.toml"))
		println("Loading test configuration file from " + confPath)
		_ = os.Chdir(appPath)
		conf.LoadFromFile(confPath)

		if noLog := os.Getenv("NOLOG"); noLog != "" {
			log.SetLevel(log.LevelError)
		}

		if err := metadataworker.EnsureTestBinary(); err != nil {
			panic(err)
		}
	})
}
