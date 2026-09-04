package publicgrpc

import (
	"net/url"
	"strings"
)

// paramValueSep splits repeated query values inside a single map entry.
// Example: params["id"] = "a\x1eb\x1ec" → id=a&id=b&id=c
const paramValueSep = "\x1e"

func mapToURLValues(m map[string]string) url.Values {
	if len(m) == 0 {
		return url.Values{}
	}
	q := make(url.Values, len(m))
	for k, v := range m {
		if strings.Contains(v, paramValueSep) {
			for _, part := range strings.Split(v, paramValueSep) {
				q.Add(k, part)
			}
			continue
		}
		q.Set(k, v)
	}
	return q
}
