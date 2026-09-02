package subsonic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/server/subsonic/responses"
)

const MinSupportedVersion = "1.8.0"

// compareSubsonicVersion returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareSubsonicVersion(a, b string) (int, error) {
	aParts, err := parseSubsonicVersion(a)
	if err != nil {
		return 0, err
	}
	bParts, err := parseSubsonicVersion(b)
	if err != nil {
		return 0, err
	}
	for i := range 3 {
		if aParts[i] < bParts[i] {
			return -1, nil
		}
		if aParts[i] > bParts[i] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseSubsonicVersion(v string) ([3]int, error) {
	var parts [3]int
	segments := strings.Split(v, ".")
	if len(segments) < 2 || len(segments) > 3 {
		return parts, fmt.Errorf("invalid subsonic version %q", v)
	}
	for i, segment := range segments {
		n, err := strconv.Atoi(segment)
		if err != nil || n < 0 {
			return parts, fmt.Errorf("invalid subsonic version %q", v)
		}
		parts[i] = n
	}
	return parts, nil
}

func validateClientVersion(clientVersion string) error {
	cmp, err := compareSubsonicVersion(clientVersion, Version)
	if err != nil {
		return nil //nolint:nilerr // tolerate malformed client version strings
	}
	if cmp > 0 {
		return newError(responses.ErrorServerTooOld)
	}
	cmp, err = compareSubsonicVersion(clientVersion, MinSupportedVersion)
	if err != nil {
		return nil //nolint:nilerr // tolerate malformed client version strings
	}
	if cmp < 0 {
		return newError(responses.ErrorClientTooOld)
	}
	return nil
}
