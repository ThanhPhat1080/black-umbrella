package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
)

func fast(t *testing.T) {
	t.Helper()
	tt, mm := ArgonTime, ArgonMemKiB
	ArgonTime, ArgonMemKiB = 1, 64
	t.Cleanup(func() { ArgonTime, ArgonMemKiB = tt, mm })
}

func TestDefaultsMatchConstraints(t *testing.T) {
	if ArgonTime != 3 || ArgonMemKiB != 64*1024 || ArgonThreads != 1 || ArgonKeyLen != 32 {
		t.Fatalf("argon defaults: %d %d %d %d", ArgonTime, ArgonMemKiB, ArgonThreads, ArgonKeyLen)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	fast(t)
	kp, _ := e2e.GenerateKeyPair()
	p := filepath.Join(t.TempDir(), "sub", "key.json")
	if err := SaveKey(p, "alice", "https://x", "123456", kp); err != nil {
		t.Fatal(err)
	}
	kf, got, err := LoadKey(p, "123456")
	if err != nil {
		t.Fatal(err)
	}
	if got != kp || kf.Username != "alice" || kf.Server != "https://x" || kf.PublicKey == "" {
		t.Fatalf("mismatch: %+v", kf)
	}
}

func TestWrongPIN(t *testing.T) {
	fast(t)
	kp, _ := e2e.GenerateKeyPair()
	p := filepath.Join(t.TempDir(), "key.json")
	_ = SaveKey(p, "alice", "s", "123456", kp)
	if _, _, err := LoadKey(p, "000000"); !errors.Is(err, ErrWrongPIN) {
		t.Fatalf("got %v", err)
	}
}

func TestFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix modes")
	}
	fast(t)
	kp, _ := e2e.GenerateKeyPair()
	p := filepath.Join(t.TempDir(), "key.json")
	_ = SaveKey(p, "a", "s", "123456", kp)
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
}

func TestExportExact(t *testing.T) {
	fast(t)
	kp, _ := e2e.GenerateKeyPair()
	d := t.TempDir()
	src, dst := filepath.Join(d, "key.json"), filepath.Join(d, "backup.json")
	_ = SaveKey(src, "a", "s", "123456", kp)
	if err := ExportKey(src, dst); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(src)
	b, _ := os.ReadFile(dst)
	if !bytes.Equal(a, b) {
		t.Fatal("bytes differ")
	}
}

func TestDirHonoursEnv(t *testing.T) {
	t.Setenv("UMBRELLA_HOME", "/tmp/xyz")
	d, err := Dir()
	if err != nil || d != "/tmp/xyz" {
		t.Fatalf("%q %v", d, err)
	}
}
