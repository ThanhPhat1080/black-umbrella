// Package links finds http(s) URLs in message text.
package links

import (
	"regexp"
	"strings"
)

var re = regexp.MustCompile(`https?://\S+`)

func balanced(s string, open, close byte) bool {
	return strings.Count(s, string(open)) >= strings.Count(s, string(close))
}

// Find returns all URLs in text, with trailing punctuation removed.
func Find(text string) []string {
	var out []string
	for _, m := range re.FindAllString(text, -1) {
		for {
			if m == "" {
				break
			}
			last := m[len(m)-1]
			trim := false
			switch last {
			case '.', ',', ';', ':', '!', '?', '\'', '"', '>':
				trim = true
			case ')':
				trim = !balanced(m, '(', ')')
			case ']':
				trim = !balanced(m, '[', ']')
			case '}':
				trim = !balanced(m, '{', '}')
			}
			if !trim {
				break
			}
			m = m[:len(m)-1]
		}
		if len(m) > len("https://") {
			out = append(out, m)
		}
	}
	return out
}
