package subsonic

import (
	"bytes"
	"encoding/json"
)

func encodeJSON(buf *bytes.Buffer, value any) error {
	// Encoder writes from encoding/json's internal buffer directly into our
	// reusable response buffer, avoiding Marshal's extra full-response allocation.
	// Retain stdlib encoding for correct embedded OpenSubsonic pointer fields.
	if err := json.NewEncoder(buf).Encode(value); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1) // Preserve the existing JSON and JSONP wire format.
	return nil
}
