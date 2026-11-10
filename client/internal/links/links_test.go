package links

import (
	"reflect"
	"testing"
)

func TestFind(t *testing.T) {
	cases := map[string][]string{
		"xem https://a.b/c?d=1, và http://x.y": {"https://a.b/c?d=1", "http://x.y"},
		"(see https://a.b/x_(y))":              {"https://a.b/x_(y)"},
		"see (https://a.b/c) ok.":              {"https://a.b/c"},
		"Link: https://a.b/ơ?q=tiếng!":         {"https://a.b/ơ?q=tiếng"},
		"no links here, ftp://x":               nil,
	}
	for in, want := range cases {
		if got := Find(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}
