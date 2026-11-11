package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
)

func TestCowMessageKeepsIndentation(t *testing.T) {
	art := cowArt("hi there")
	if !strings.Contains(art, "< hi there >") {
		t.Fatalf("speech bubble missing:\n%s", art)
	}
	msg := chat.Msg{ID: "c1", Sender: "bob", Text: chat.Art(art), CreatedAt: time.Now()}
	var plain []string
	for _, l := range renderMessage(msg, msgView{Name: "Bob", Me: "me", Width: 60}) {
		plain = append(plain, ansi.Strip(l))
	}
	all := strings.Join(plain, "\n")
	if !strings.Contains(all, "                ||----w |") || !strings.Contains(all, `\  (oo)\_______`) {
		t.Fatalf("art lost its spaces:\n%s", all)
	}
	for _, l := range plain {
		if width(l) > 60 {
			t.Fatalf("line too wide (%d): %q", width(l), l)
		}
	}
	if cowArt("") == "" || !strings.Contains(cowArt("  "), "Moo!") {
		t.Fatal("default cow text")
	}
	if got := cowArt(strings.Repeat("x", 100)); strings.Contains(got, strings.Repeat("x", 41)) {
		t.Fatal("cow text must be capped")
	}
	if text, ok := chat.Preformatted("plain"); ok || text != "" {
		t.Fatal("plain text is not art")
	}
}

func TestDanceMessageShowsAnimalAndAnimates(t *testing.T) {
	msg := chat.Msg{ID: "d1", Sender: "bob", Text: chat.DanceText, CreatedAt: time.Now()}
	rest := strings.Join(renderMessage(msg, msgView{Name: "Bob", Me: "me", Width: 60}), "\n")
	if !strings.Contains(ansi.Strip(rest), "Bob is dancing!") {
		t.Fatalf("dance banner missing:\n%s", ansi.Strip(rest))
	}
	a0 := strings.Join(danceBody("d1", "Bob", buzzAnim{Active: true, Elapsed: 0}), "\n")
	a1 := strings.Join(danceBody("d1", "Bob", buzzAnim{Active: true, Elapsed: danceStep}), "\n")
	if ansi.Strip(a0) == ansi.Strip(a1) {
		t.Fatal("dancer must change pose between frames")
	}
	var d danceState
	t0 := time.Now()
	d.begin("d1", t0)
	if !d.anim("d1", t0.Add(time.Second)).Active || d.anim("d1", t0.Add(danceDur)).Active {
		t.Fatal("dance animates for danceDur only")
	}
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} { // every animal fits and has 2 poses
		for _, f := range []buzzAnim{{}, {Active: true, Elapsed: danceStep}} {
			for _, l := range danceBody(id, "N", f) {
				if width(l) > 40 {
					t.Fatalf("animal too wide: %q", ansi.Strip(l))
				}
			}
		}
	}
}

func TestDanceCowAndPetCommands(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	typeAndEnter(m, "/dance")
	typeAndEnter(m, "/dance") // cooldown
	if n := len(f.sends()); n != 1 {
		t.Fatalf("/dance sends: %d, want 1 (second is on cooldown)", n)
	}
	typeAndEnter(m, "/cow moo moo")
	if n := len(f.sends()); n != 2 {
		t.Fatalf("/cow should send one message, sends=%d", n)
	}
	if m.input.Value() != "" {
		t.Fatal("input should be cleared after /cow")
	}

	if !strings.Contains(strings.Join(m.petLines(24, 33), ""), "/\\_/\\") {
		t.Fatal("pet should be visible by default")
	}
	typeAndEnter(m, "/e")
	if !m.petOff || m.petLines(24, 33) != nil {
		t.Fatal("/e should hide the pet")
	}
	typeAndEnter(m, "/e")
	if m.petOff || m.petLines(24, 33) == nil {
		t.Fatal("/e again should show the pet")
	}
}

func TestCowAndDanceAreCentered(t *testing.T) {
	const w = 80
	for _, text := range []string{chat.Art(cowArt("moo")), chat.DanceText} {
		for _, sender := range []string{"me", "bob"} {
			m := chat.Msg{ID: "o1", Sender: sender, Text: text, CreatedAt: time.Now()}
			lines := renderMessage(m, msgView{Name: "N", Me: "me", Width: w})
			left, right := w, 0 // leftmost / rightmost drawn cell over all lines
			h := strings.TrimRight(ansi.Strip(lines[0]), " ")
			if sender == "me" && (!strings.HasSuffix(h, "N ○") || width(h) != w-1) {
				t.Fatalf("our header must hug the right edge: %q (w=%d)", h, width(h))
			}
			if sender == "bob" && !strings.HasPrefix(h, " ○ N") {
				t.Fatalf("their header must stay at the left edge: %q", h)
			}
			if strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "[") {
				t.Fatalf("%s/%s must have no buttons:\n%s", sender, text[:3], ansi.Strip(strings.Join(lines, "\n")))
			}
			for _, l := range lines[1:] { // the art only: no header
				p := ansi.Strip(l)
				if t := strings.TrimLeft(p, " "); t != "" {
					left = minInt(left, width(p)-width(t))
					right = maxInt(right, width(strings.TrimRight(p, " ")))
				}
			}
			if d := left - (w - right); d < -3 || d > 3 || left < 10 {
				t.Fatalf("%s/%s not centred: left margin %d, right margin %d\n%s", sender, text[:3], left, w-right, ansi.Strip(strings.Join(lines, "\n")))
			}
		}
	}
	// regular own messages stay flush right
	m := chat.Msg{ID: "o2", Sender: "me", Text: "hi", CreatedAt: time.Now()}
	if h := ansi.Strip(renderMessage(m, msgView{Name: "Me", Me: "me", Width: w})[0]); !strings.HasSuffix(strings.TrimRight(h, " "), "Me ○") {
		t.Fatalf("regular own message lost its alignment: %q", h)
	}
}

func TestCowsayBubbles(t *testing.T) {
	one := strings.Split(cowArt("Moo!"), "\n")
	if one[0] != " ______" || one[1] != "< Moo! >" || one[2] != " ------" || one[3] != `        \   ^__^` {
		t.Fatalf("one-line bubble:\n%s", strings.Join(one, "\n"))
	}
	many := strings.Split(cowArt(strings.Repeat("hello world ", 8)), "\n")
	if !strings.HasPrefix(many[1], "/ ") || !strings.HasSuffix(many[1], " \\") || !strings.HasPrefix(many[2], "| ") && !strings.HasPrefix(many[2], "\\ ") {
		t.Fatalf("multi-line bubble:\n%s", strings.Join(many, "\n"))
	}
	w := width(many[1])
	for _, l := range many[1 : len(many)-6] {
		if width(l) != w {
			t.Fatalf("bubble rows must have equal width: %q", l)
		}
	}
}

func TestTalkingAnimalsSayTheText(t *testing.T) {
	for _, kind := range []string{"cat", "rabbit", "dog", "duck"} {
		art, ok := talkArt(kind, "xin chào")
		if !ok || !strings.Contains(art, "< xin chào >") || strings.Count(art, "\n") < 6 {
			t.Fatalf("%s: ok=%v\n%s", kind, ok, art)
		}
		if def, _ := talkArt(kind, ""); !strings.Contains(def, talkers[kind].def) {
			t.Fatalf("%s: empty text should say its default", kind)
		}
	}
	if _, ok := talkArt("cow", "x"); ok {
		t.Fatal("cow is not in the table")
	}
}

func TestFishSwimForTenSecondsThenRest(t *testing.T) {
	var d danceState
	t0 := time.Now()
	d.begin("f1", t0)
	a, b := fishBody("bob", 40, d.anim("f1", t0.Add(time.Second))), fishBody("bob", 40, d.anim("f1", t0.Add(2*time.Second)))
	if strings.Join(a, "\n") == strings.Join(b, "\n") {
		t.Fatal("fish should move")
	}
	if !d.anim("f1", t0.Add(9*time.Second)).Active || d.anim("f1", t0.Add(11*time.Second)).Active {
		t.Fatal("fish swim for about ten seconds")
	}
	if len(a) != 1+len(fishRows) {
		t.Fatalf("rows = %d", len(a))
	}
}

func TestBuzzAndDanceLastTenSeconds(t *testing.T) {
	if buzzTextDur != 10*time.Second || danceDur != 10*time.Second {
		t.Fatalf("buzz %v dance %v", buzzTextDur, danceDur)
	}
	var b buzzState
	t0 := time.Now()
	b.begin("x", t0)
	if b.shake(t0.Add(6*time.Second+100*time.Millisecond)) == 0 {
		t.Fatal("the screen should shake again in a later burst")
	}
	if b.shake(t0.Add(11*time.Second)) != 0 {
		t.Fatal("no shake after ten seconds")
	}
}
