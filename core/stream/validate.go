package stream

import (
	"strings"

	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/log"
)

// ValidateDefaultDownsamplingFormat logs a startup warning when the configured
// default downsampling format has no built-in or DB-backed transcoding command.
func ValidateDefaultDownsamplingFormat(format string) {
	format = strings.TrimSpace(strings.ToLower(format))
	if format == "" {
		return
	}
	for _, dt := range consts.DefaultTranscodings {
		if strings.EqualFold(dt.TargetFormat, format) {
			return
		}
	}
	log.Warn("DefaultDownsamplingFormat is set to an unsupported built-in format",
		"format", format,
		"supported", supportedBuiltinFormats())
}

func supportedBuiltinFormats() []string {
	formats := make([]string, 0, len(consts.DefaultTranscodings))
	for _, dt := range consts.DefaultTranscodings {
		formats = append(formats, dt.TargetFormat)
	}
	return formats
}
