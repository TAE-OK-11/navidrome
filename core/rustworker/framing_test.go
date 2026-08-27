package rustworker

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestWriteJSONLineReadJSONLine(t *testing.T) {
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	if err := WriteJSONLine(writer, map[string]any{"op": "ping"}); err != nil {
		t.Fatalf("WriteJSONLine: %v", err)
	}
	var decoded map[string]any
	if err := ReadJSONLine(bufio.NewReader(&buf), &decoded); err != nil {
		t.Fatalf("ReadJSONLine: %v", err)
	}
	if decoded["op"] != "ping" {
		t.Fatalf("decoded op = %v, want ping", decoded["op"])
	}
}

func TestWriteHeaderAndBodies(t *testing.T) {
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	body := []byte("payload")
	if err := WriteHeaderAndBodies(writer, map[string]int{"size": len(body)}, body); err != nil {
		t.Fatalf("WriteHeaderAndBodies: %v", err)
	}
	raw := buf.String()
	line, rest, ok := strings.Cut(raw, "\n")
	if !ok {
		t.Fatal("missing newline after header")
	}
	if line != `{"size":7}` {
		t.Fatalf("header line = %q", line)
	}
	if rest != "payload" {
		t.Fatalf("body = %q, want payload", rest)
	}
}

func TestReadSizedBody(t *testing.T) {
	data, err := ReadSizedBody(strings.NewReader("hello"), 5, 10)
	if err != nil {
		t.Fatalf("ReadSizedBody: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("data = %q", data)
	}
}
