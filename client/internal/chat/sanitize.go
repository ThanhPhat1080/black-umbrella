package chat

import (
	"regexp"
	"strings"
)

var (
	oscRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?`)
	dcsRe = regexp.MustCompile(`\x1b[PX^_][^\x1b]*(?:\x1b\\)?`)
	csiRe = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

// Sanitize makes untrusted text safe to print to a terminal: it removes ANSI escape sequences
// and C0/C1 control characters (newlines are kept, tabs become spaces) and bidi overrides.
func Sanitize(s string) string {
	s = oscRe.ReplaceAllString(s, "")
	s = dcsRe.ReplaceAllString(s, "")
	s = csiRe.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteString("    ")
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x2028, r == 0x2029:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SanitizeLine is Sanitize for single-line fields (names, status text): newlines become spaces.
func SanitizeLine(s string) string {
	return strings.ReplaceAll(Sanitize(s), "\n", " ")
}
