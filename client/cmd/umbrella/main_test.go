package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/store"
)

func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "umbrella")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func run(t *testing.T, bin, home, stdin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(cmd.Environ(), "UMBRELLA_HOME="+home)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return out.String(), code
}

// Review focus 4: a wrong PIN is rejected locally, before any network call
// (the server below is unreachable, yet the message is "Wrong PIN.").
func TestWrongPINIsLocal(t *testing.T) {
	bin := build(t)
	home := t.TempDir()
	kp, _ := e2e.GenerateKeyPair()
	if err := store.SaveKey(filepath.Join(home, "key.json"), "alice", "http://127.0.0.1:1", "123456", kp); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, bin, home, "000000\n")
	if code != 1 || !strings.Contains(out, "Wrong PIN.") || strings.Contains(out, "reach") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	// right PIN proceeds to the network and fails there
	out, code = run(t, bin, home, "123456\n")
	if code != 1 || !strings.Contains(out, "Can't reach the server") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestNoKeyAndHelp(t *testing.T) {
	bin := build(t)
	out, code := run(t, bin, t.TempDir(), "")
	if code != 1 || !strings.Contains(out, "umbrella register") {
		t.Fatalf("%d %q", code, out)
	}
	out, code = run(t, bin, t.TempDir(), "", "--help")
	if code != 0 || !strings.Contains(out, "export-key") || !strings.Contains(out, "admin invite") {
		t.Fatalf("%d %q", code, out)
	}
}

func TestRegisterRefusesOverwrite(t *testing.T) {
	bin := build(t)
	home := t.TempDir()
	kp, _ := e2e.GenerateKeyPair()
	_ = store.SaveKey(filepath.Join(home, "key.json"), "alice", "s", "123456", kp)
	out, code := run(t, bin, home, "", "register")
	if code != 1 || !strings.Contains(out, "--force") {
		t.Fatalf("%d %q", code, out)
	}
}
