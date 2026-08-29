package ftsnormalize

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var ftsPunctStrip = regexp.MustCompile(`[^\p{L}\p{N}]`)

// normalizeInProcess mirrors rust/fts-normalize normalize_for_fts without a subprocess.
func normalizeInProcess(values ...string) string {
	if len(values) == 0 {
		return ""
	}
	seen := make(map[string]struct{})
	var result []string
	add := func(orig, variant string) {
		if variant == "" || variant == orig {
			return
		}
		lower := strings.ToLower(variant)
		if _, ok := seen[lower]; ok {
			return
		}
		seen[lower] = struct{}{}
		result = append(result, variant)
	}

	for _, value := range values {
		for _, word := range strings.Fields(value) {
			transliterated := asciiFold(word)
			add(word, ftsPunctStrip.ReplaceAllString(transliterated, ""))
			add(word, transliterated)
		}
	}
	return strings.Join(result, " ")
}

func asciiFold(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch r {
		case 'Ø':
			builder.WriteByte('O')
		case 'ø':
			builder.WriteByte('o')
		case 'Æ':
			builder.WriteString("AE")
		case 'æ':
			builder.WriteString("ae")
		case 'Œ':
			builder.WriteString("OE")
		case 'œ':
			builder.WriteString("oe")
		case 'ß':
			builder.WriteString("ss")
		case 'Ð':
			builder.WriteByte('D')
		case 'ð':
			builder.WriteByte('d')
		case 'Þ':
			builder.WriteString("TH")
		case 'þ':
			builder.WriteString("th")
		case 'Ł':
			builder.WriteByte('L')
		case 'ł':
			builder.WriteByte('l')
		default:
			builder.WriteRune(r)
		}
	}
	return stripCombiningMarks(builder.String())
}

func stripCombiningMarks(value string) string {
	if value == "" {
		return value
	}
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}), norm.NFC)
	out, _, err := transform.String(t, value)
	if err != nil {
		return value
	}
	return out
}
