package scrobbler

import (
	"testing"
	"time"
)

func BenchmarkNowPlayingWakeTimer(b *testing.B) {
	b.Run("time-after", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			<-time.After(0)
		}
	})
	b.Run("reused-timer", func(b *testing.B) {
		timer := time.NewTimer(0)
		defer timer.Stop()
		b.ReportAllocs()
		for b.Loop() {
			<-timer.C
			timer.Reset(0)
		}
	})
}
