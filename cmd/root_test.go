package cmd

import (
	"github.com/navidrome/navidrome/conf"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("schedulerRequired", func() {
	var (
		pluginsEnabled  bool
		devOptimizeDB   bool
		backupSchedule  string
		scannerEnabled  bool
		scannerSchedule string
	)

	BeforeEach(func() {
		pluginsEnabled = conf.Server.Plugins.Enabled
		devOptimizeDB = conf.Server.DevOptimizeDB
		backupSchedule = conf.Server.Backup.Schedule
		scannerEnabled = conf.Server.Scanner.Enabled
		scannerSchedule = conf.Server.Scanner.Schedule

		conf.Server.Plugins.Enabled = false
		conf.Server.DevOptimizeDB = false
		conf.Server.Backup.Schedule = ""
		conf.Server.Scanner.Enabled = true
		conf.Server.Scanner.Schedule = ""
	})

	AfterEach(func() {
		conf.Server.Plugins.Enabled = pluginsEnabled
		conf.Server.DevOptimizeDB = devOptimizeDB
		conf.Server.Backup.Schedule = backupSchedule
		conf.Server.Scanner.Enabled = scannerEnabled
		conf.Server.Scanner.Schedule = scannerSchedule
	})

	It("skips the scheduler when no scheduled services are enabled", func() {
		Expect(schedulerRequired()).To(BeFalse())
	})

	It("starts the scheduler for plugin scheduling support", func() {
		conf.Server.Plugins.Enabled = true
		Expect(schedulerRequired()).To(BeTrue())
	})

	It("starts the scheduler for periodic scans", func() {
		conf.Server.Scanner.Schedule = "@every 1h"
		Expect(schedulerRequired()).To(BeTrue())
	})

	It("starts the scheduler for periodic backups", func() {
		conf.Server.Backup.Schedule = "@every 24h"
		Expect(schedulerRequired()).To(BeTrue())
	})

	It("starts the scheduler for DB optimization", func() {
		conf.Server.DevOptimizeDB = true
		Expect(schedulerRequired()).To(BeTrue())
	})
})
