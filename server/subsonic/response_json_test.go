package subsonic

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestResponseJSONPreservesWireFormat(t *testing.T) {
	for _, value := range []any{benchmarkJSONResponse, map[string]string{"title": "한국어 <script> & \u2028"}, nil} {
		expected, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		buf := bytes.NewBufferString("callback(")
		if err := encodeJSON(buf, value); err != nil {
			t.Fatal(err)
		}
		expected = append([]byte("callback("), expected...)
		if !bytes.Equal(buf.Bytes(), expected) {
			t.Fatal("JSON bytes differ from Marshal, including JSONP prefix")
		}
	}
}

func BenchmarkSubsonicResponseBuffer(b *testing.B) {
	b.Run("marshal_then_copy", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		for b.Loop() {
			buf.Reset()
			data, err := json.Marshal(benchmarkJSONResponse)
			if err != nil {
				b.Fatal(err)
			}
			buf.Write(data)
		}
	})
	b.Run("encode_into_buffer", func(b *testing.B) {
		var buf bytes.Buffer
		b.ReportAllocs()
		for b.Loop() {
			buf.Reset()
			if err := encodeJSON(&buf, benchmarkJSONResponse); err != nil {
				b.Fatal(err)
			}
		}
	})
}
