package log

import "testing"

var benchmarkLegacyLevel = LevelInfo

func BenchmarkDisabledLevelCheck(b *testing.B) {
	previous := CurrentLevel()
	SetLevel(LevelInfo)
	SetLogLevels(nil)
	b.Cleanup(func() {
		SetLevel(previous)
		SetLogLevels(nil)
	})
	b.Run("atomic-fast-path", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = IsGreaterOrEqualTo(LevelDebug)
		}
	})
	b.Run("legacy-rwmutex", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			loggerMu.RLock()
			level := benchmarkLegacyLevel
			levels := logLevels
			loggerMu.RUnlock()
			_ = level >= LevelDebug || len(levels) != 0
		}
	})
}
