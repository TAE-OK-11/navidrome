package events

import (
	"context"
	"net/http/httptest"
	"testing"
)

func BenchmarkSSEWriteEvent(b *testing.B) {
	broker := &broker{}
	msg := broker.prepareMessage(context.Background(), &KeepAlive{TS: 1_700_000_000})
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	for b.Loop() {
		msg.id++
		if err := writeEvent(context.Background(), rec, msg, writeTimeOut); err != nil {
			b.Fatal(err)
		}
		rec.Body.Reset()
	}
}
