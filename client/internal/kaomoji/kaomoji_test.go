package kaomoji

import "testing"

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
