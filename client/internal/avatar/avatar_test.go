package avatar

import (
	"regexp"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestGridDeterministicSymmetric(t *testing.T) {
	for _, u := range []string{"alice", "bob", "thanh_phat", "zed99"} {
		g1, c1 := Grid(u)
		g2, c2 := Grid(u)
		if g1 != g2 || c1 != c2 {
			t.Fatal("not deterministic")
		}
		if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(c1) {
			t.Fatalf("colour %q", c1)
		}
		for r := 0; r < 5; r++ {
			for c := 0; c < 5; c++ {
				if g1[r][c] != g1[r][4-c] {
					t.Fatalf("%s not symmetric", u)
				}
			}
		}
	}
	a, _ := Grid("alice")
	b, _ := Grid("bob")
	if a == b {
		t.Fatal("alice and bob collide")
	}
}

func TestRenderShape(t *testing.T) {
	lines := Render("alice")
	if len(lines) != 3 {
		t.Fatalf("%d lines", len(lines))
	}
	for _, l := range lines {
		if w := runewidth.StringWidth(l); w != 10 {
			t.Fatalf("width %d for %q", w, l)
		}
	}
}

func TestDot(t *testing.T) {
	if d := Dot("alice"); !regexp.MustCompile("■").MatchString(d) {
		t.Fatal(d)
	}
}
