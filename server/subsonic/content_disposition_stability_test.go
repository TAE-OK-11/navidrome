package subsonic

import (
	"mime"
	"strings"
	"testing"
)

func TestAttachmentDispositionSupportsUnicodeAndQuotes(t *testing.T) {
	original := "한글 \"mix\".flac"
	value := attachmentDisposition(original)
	if strings.ContainsAny(value, "\r\n") {
		t.Fatalf("header contains a line break: %q", value)
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		t.Fatalf("invalid Content-Disposition: %v (%q)", err, value)
	}
	if mediaType != "attachment" || params["filename"] != original {
		t.Fatalf("unexpected parsed disposition: %q %#v", mediaType, params)
	}
}

func TestAttachmentDispositionStripsHeaderControlCharacters(t *testing.T) {
	value := attachmentDisposition("track\r\nX-Evil: yes.flac")
	if strings.ContainsAny(value, "\r\n") {
		t.Fatalf("header injection characters survived: %q", value)
	}
}
