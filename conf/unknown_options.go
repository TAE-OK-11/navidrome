package conf

import (
	"cmp"
	"encoding"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/utils/slice"
	"github.com/spf13/viper"
)

// Extra keys that are valid but live outside configOptions (HTTP/3 is viper-backed).
var extraKnownConfigKeys = []string{
	"enablehttp3",
	"http3allow0rtt",
	"http3provider",
	"http3gatewaypath",
	"http3altsvcmaxage",
	"http3qlogdir",
	"http3maxconnections",
	"http3maxconnectionsperip",
	"http3maxinflightrequests",
	"http3connectionratepersecond",
	"http3connectionburst",
	"http3congestioncontrol",
}

func logUnknownOptions() {
	unknown := unknownConfigKeys()
	if len(unknown) == 0 {
		return
	}
	for _, key := range unknown {
		if suggestions := suggestOptions(key); len(suggestions) > 0 {
			log.Warn("Unrecognized config option", "option", key, "suggestions", suggestions)
			continue
		}
		log.Warn("Unrecognized config option", "option", key)
	}
}

func suggestOptions(key string) []string {
	key = strings.ToLower(key)
	leaf := leafKey(key)
	canonical, _ := configKeys()
	var matches []string
	for known, name := range canonical {
		if known != key && leafKey(known) == leaf {
			matches = append(matches, name)
		}
	}
	slices.Sort(matches)
	return matches
}

func leafKey(key string) string {
	return key[strings.LastIndex(key, ".")+1:]
}

func unknownConfigKeys() []string {
	skipDefault := strings.EqualFold(filepath.Ext(viper.ConfigFileUsed()), ".ini")
	var unknown []string
	for _, key := range viper.AllKeys() {
		if !viper.InConfig(key) || canonicalOptionName(key) != "" {
			continue
		}
		if skipDefault && strings.HasPrefix(key, "default.") {
			continue
		}
		if strings.HasPrefix(key, "nd_") && canonicalOptionName(ndKeyToCanonical(key)) != "" {
			continue
		}
		unknown = append(unknown, key)
	}
	slices.Sort(unknown)
	return asWrittenInConfigFile(unknown)
}

func ndKeyToCanonical(key string) string {
	return strings.ReplaceAll(strings.TrimPrefix(key, "nd_"), "_", ".")
}

func canonicalOptionName(key string) string {
	keys, prefixes := configKeys()
	if name, ok := keys[key]; ok {
		return name
	}
	if slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(key, p) }) {
		return toPascalCase(key)
	}
	return ""
}

func asWrittenInConfigFile(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	data, err := os.ReadFile(viper.ConfigFileUsed())
	if err != nil {
		return keys
	}
	casing := map[string]string{}
	for _, match := range configFileKeyRx.FindAllStringSubmatch(string(data), -1) {
		for segment := range strings.SplitSeq(match[1], ".") {
			lower := strings.ToLower(segment)
			casing[lower] = cmp.Or(casing[lower], segment)
		}
	}
	return slice.Map(keys, func(key string) string {
		segments := strings.Split(key, ".")
		for i, s := range segments {
			segments[i] = cmp.Or(casing[s], s)
		}
		return strings.Join(segments, ".")
	})
}

var configFileKeyRx = regexp.MustCompile(`(?m)^\s*\[?\s*"?([\w.]+)"?\s*[]=:]`)

var configKeys = sync.OnceValues(func() (map[string]string, []string) {
	keys := map[string]string{}
	var prefixes []string

	var collect func(t reflect.Type, prefix string)
	collect = func(t reflect.Type, prefix string) {
		for i := range t.NumField() {
			field := t.Field(i)
			if !field.IsExported() || field.Tag.Get("conf") == "-" {
				continue
			}
			name := prefix + field.Name
			if field.Type.Kind() == reflect.Struct && !reflect.PointerTo(field.Type).Implements(textUnmarshalerType) {
				collect(field.Type, name+".")
				continue
			}
			lower := strings.ToLower(name)
			keys[lower] = name
			if field.Type.Kind() == reflect.Map {
				prefixes = append(prefixes, lower+".")
			}
		}
	}
	collect(reflect.TypeFor[configOptions](), "")
	for _, extra := range extraKnownConfigKeys {
		keys[extra] = toPascalCase(extra)
	}
	return keys, prefixes
})

var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
