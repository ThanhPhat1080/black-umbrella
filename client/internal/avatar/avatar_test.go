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

func TestNameColorStableAndSpread(t *testing.T) {
	if NameColor("alice") != NameColor("alice") {
		t.Fatal("not stable")
	}
	names := []string{"alice", "bob", "carol", "dave", "erin", "frank", "grace", "heidi", "ivan", "judy"}
	seen := map[int]bool{}
	for _, n := range names {
		seen[int(Hue(n)/15)] = true // 15-degree buckets
	}
	if len(seen) < 6 {
		t.Fatalf("only %d distinct hues among %d names", len(seen), len(names))
	}
	c := NameColor("alice")
	if c.Dark != Color("alice") {
		t.Fatalf("dark variant %s must match the avatar colour %s", c.Dark, Color("alice"))
	}
	if c.Light == c.Dark || c.Light == "" {
		t.Fatalf("light variant %q", c.Light)
	}
}

func TestMini(t *testing.T) {
	names := []string{"alice", "bob", "carol", "dave", "erin", "frank", "grace", "heidi", "ivan", "judy", "mallory", "trent"}
	seen := map[string]bool{}
	for _, n := range names {
		m := Mini(n)
		if m != Mini(n) {
			t.Fatal("not deterministic")
		}
		if w := runewidth.StringWidth(m); w != 3 {
			t.Fatalf("%s: width %d (%q)", n, w, m)
		}
		seen[m] = true
	}
	if len(seen) < 9 {
		t.Fatalf("only %d distinct minis among %d users: %v", len(seen), len(names), seen)
	}
	// symmetric: cell 0 mirrors cell 2
	g, _ := Grid("alice")
	_ = g
	r := []rune(Mini("alice"))
	if len(r) != 3 || mirror(r[0]) != r[2] {
		t.Fatalf("not left/right symmetric: %q", string(r))
	}
}

func mirror(r rune) rune {
	for l, rr := range map[rune]rune{'▘': '▝', '▝': '▘', '▖': '▗', '▗': '▖', '▌': '▐', '▐': '▌', '▞': '▚', '▚': '▞', '▛': '▜', '▜': '▛', '▙': '▟', '▟': '▙'} {
		if l == r {
			return rr
		}
	}
	return r
}
