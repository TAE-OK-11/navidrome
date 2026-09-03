package integration

import (
	"crypto/md5"
	"encoding/hex"
	"slices"
	"strings"
)

func signAudioscrobbler(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "format" || k == "callback" || k == "api_sig" {
			continue
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var msg strings.Builder
	for _, k := range keys {
		msg.WriteString(k)
		msg.WriteString(params[k])
	}
	msg.WriteString(secret)
	sum := md5.Sum([]byte(msg.String()))
	return hex.EncodeToString(sum[:])
}
