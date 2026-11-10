package kaomoji

import (
	"testing"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func TestAll(t *testing.T) {
	if len(All) != 50 {
		t.Fatalf("len %d", len(All))
	}
	seen := map[string]bool{}
	for _, k := range All {
		if k == "" || seen[k] {
			t.Fatalf("empty or duplicate %q", k)
		}
		seen[k] = true
	}
}

func TestSafeForTerminals(t *testing.T) {
	for _, k := range All {
		if w1, w2 := runewidth.StringWidth(k), lipgloss.Width(k); w1 != w2 {
			t.Errorf("%q: runewidth %d != lipgloss %d", k, w1, w2)
		}
		for _, r := range k {
			switch {
			case unicode.In(r, unicode.Mn, unicode.Mc, unicode.Me):
				t.Errorf("%q has combining mark %U", k, r)
			case r >= 0xff61 && r <= 0xff9f:
				t.Errorf("%q has halfwidth kana/punctuation %U", k, r)
			case r == 0x2727:
				t.Errorf("%q has U+2727", k)
			case r < 0x20 || (r >= 0x7f && r <= 0x9f):
				t.Errorf("%q has control char", k)
			}
		}
	}
}
