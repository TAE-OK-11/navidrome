package cmd

import (
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/stream/hotcache"
	"github.com/navidrome/navidrome/log"
	"github.com/spf13/cobra"
)

func init() {
	run := rootCmd.Run
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		secureConfiguredHotCachePath()
		if run != nil {
			run(cmd, args)
		}
	}
}

func secureConfiguredHotCachePath() {
	if err := hotcache.PrepareConfiguredPath(); err != nil {
		conf.Server.HotCache.Enabled = false
		log.Warn("Original hot cache disabled; refusing unsafe cache directory", err)
	}
}
