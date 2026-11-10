package e2e

import (
	"errors"
	"strings"
	"testing"
)

func mustKP(t *testing.T) KeyPair {
	t.Helper()
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return kp
}

func TestRoundTripThreeRecipients(t *testing.T) {
	a, b, c := mustKP(t), mustKP(t), mustKP(t)
	rec := map[string][32]byte{"a": a.Public, "b": b.Public, "c": c.Public}
	s, err := Seal("hello", a, rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Keys) != 3 {
		t.Fatalf("keys: %d", len(s.Keys))
	}
	for name, kp := range map[string]KeyPair{"a": a, "b": b, "c": c} {
		got, err := Open(s.Nonce, s.CT, s.Keys[name], kp, a.Public)
		if err != nil || got != "hello" {
			t.Fatalf("%s: %q %v", name, got, err)
		}
	}
}

func TestWrongSenderAndTamper(t *testing.T) {
	a, b, evil := mustKP(t), mustKP(t), mustKP(t)
	s, _ := Seal("x", a, map[string][32]byte{"b": b.Public})
	if _, err := Open(s.Nonce, s.CT, s.Keys["b"], b, evil.Public); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong sender: %v", err)
	}
	ct := []byte(s.CT)
	if ct[4] == 'A' {
		ct[4] = 'B'
	} else {
		ct[4] = 'A'
	}
	if _, err := Open(s.Nonce, string(ct), s.Keys["b"], b, a.Public); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := Open(s.Nonce, s.CT, "!!notbase64", b, a.Public); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("garbage key: %v", err)
	}
}

func TestUnicodeByteExact(t *testing.T) {
	a := mustKP(t)
	text := "Tiếng Việt có dấu ạ ( ˘ ³˘)♥"
	s, _ := Seal(text, a, map[string][32]byte{"a": a.Public})
	got, err := Open(s.Nonce, s.CT, s.Keys["a"], a, a.Public)
	if err != nil || got != text {
		t.Fatalf("%q %v", got, err)
	}
}

func TestSealDoesNotHTMLEscapeAndFitsLimit(t *testing.T) {
	a := mustKP(t)
	text := strings.Repeat("<", 4000)
	s, err := Seal(text, a, map[string][32]byte{"a": a.Public})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.CT) > 7000 { // 4000 raw bytes + JSON + tag, base64; HTML escaping would give ~32000
		t.Fatalf("ct %d chars", len(s.CT))
	}
	if got, err := Open(s.Nonce, s.CT, s.Keys["a"], a, a.Public); err != nil || got != text {
		t.Fatal(err)
	}
	emoji := strings.Repeat("🌂", 4000)
	s, _ = Seal(emoji, a, map[string][32]byte{"a": a.Public})
	if len(s.CT) > 32768 {
		t.Fatalf("emoji ct %d chars exceeds server limit", len(s.CT))
	}
}
