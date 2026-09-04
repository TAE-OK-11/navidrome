package subsonic

import (
	"path"
	"strings"
)

var openStreamEndpoints = map[string]struct{}{
	"stream":             {},
	"download":           {},
	"getcoverart":        {},
	"getavatar":          {},
	"gettranscodestream": {},
}

func openEndpointAllowed(endpoint string) bool {
	endpoint = strings.ToLower(strings.TrimSuffix(path.Base(endpoint), ".view"))
	_, ok := openStreamEndpoints[endpoint]
	return ok
}
