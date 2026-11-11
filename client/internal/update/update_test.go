package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, cur string
		want        bool
	}{
		{"v1.2.0", "v1.1.0", true},
		{"v1.1.0", "v1.1.0", false},
		{"v1.0.9", "v1.1.0", false},
		{"v1.10.0", "v1.9.0", true},
		{"v1.2.0", "dev", true},
		{"v1.2.0", "", true},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.cur); got != c.want {
			t.Errorf("Newer(%q,%q)=%v want %v", c.latest, c.cur, got, c.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	if got := assetName("windows", "amd64"); got != "umbrella-windows-amd64.exe" {
		t.Error(got)
	}
	if got := assetName("linux", "arm64"); got != "umbrella-linux-arm64" {
		t.Error(got)
	}
}

func TestApply(t *testing.T) {
	bin := []byte("new-binary")
	sum := sha256.Sum256(bin)
	name := assetName("linux", "amd64")
	mux := http.NewServeMux()
	mux.HandleFunc("/bin", func(w http.ResponseWriter, _ *http.Request) { w.Write(bin) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s\n%s  other\n", hex.EncodeToString(sum[:]), name, "00")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "umbrella")
	os.WriteFile(exe, []byte("old"), 0o755)
	rel := Release{Tag: "v9.9.9", Assets: map[string]string{name: srv.URL + "/bin", "SHA256SUMS.txt": srv.URL + "/sums"}}
	if err := apply(context.Background(), rel, "linux", "amd64", exe); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new-binary" {
		t.Fatalf("exe = %q", got)
	}

	// checksum mismatch must leave the old binary untouched
	os.WriteFile(exe, []byte("old"), 0o755)
	rel.Assets["SHA256SUMS.txt"] = srv.URL + "/bad"
	mux.HandleFunc("/bad", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, "%s  %s\n", "deadbeef", name) })
	if err := apply(context.Background(), rel, "linux", "amd64", exe); err == nil {
		t.Fatal("expected checksum error")
	}
	got, _ = os.ReadFile(exe)
	if string(got) != "old" {
		t.Fatalf("exe changed on bad checksum: %q", got)
	}
}

func TestApplyReportsPermissionProblemAsNeedsRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	bin := []byte("new")
	sum := sha256.Sum256(bin)
	name := assetName("linux", "amd64")
	mux := http.NewServeMux()
	mux.HandleFunc("/bin", func(w http.ResponseWriter, _ *http.Request) { w.Write(bin) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := t.TempDir()
	exe := filepath.Join(dir, "umbrella")
	os.WriteFile(exe, []byte("old"), 0o755)
	os.Chmod(dir, 0o555) // like /usr/local/bin for a normal user
	defer os.Chmod(dir, 0o755)
	rel := Release{Tag: "v9.9.9", Assets: map[string]string{name: srv.URL + "/bin", "SHA256SUMS.txt": srv.URL + "/sums"}}
	err := apply(context.Background(), rel, "linux", "amd64", exe)
	if err == nil || !NeedsRoot(err) {
		t.Fatalf("want a permission error, got %v", err)
	}
	if NeedsRoot(errors.New("boom")) {
		t.Fatal("other errors are not permission problems")
	}
}

// The Windows branch can't replace a running .exe, so it renames it aside first; check that dance.
func TestApplyWindowsKeepsOldBinaryAsDotOld(t *testing.T) {
	bin := []byte("new-exe")
	sum := sha256.Sum256(bin)
	name := assetName("windows", "amd64")
	if name != "umbrella-windows-amd64.exe" {
		t.Fatal(name)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/bin", func(w http.ResponseWriter, _ *http.Request) { w.Write(bin) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := t.TempDir()
	exe := filepath.Join(dir, "umbrella.exe")
	os.WriteFile(exe, []byte("old-exe"), 0o755)
	os.WriteFile(exe+".old", []byte("stale"), 0o755) // leftover from an earlier update
	rel := Release{Tag: "v9.9.9", Assets: map[string]string{name: srv.URL + "/bin", "SHA256SUMS.txt": srv.URL + "/sums"}}
	if err := apply(context.Background(), rel, "windows", "amd64", exe); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new-exe" {
		t.Fatalf("exe = %q", got)
	}
	if got, _ := os.ReadFile(exe + ".old"); string(got) != "old-exe" {
		t.Fatalf(".old = %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".umbrella-update-*")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
}

func TestApplyPicksNoBuildForUnsupportedPlatform(t *testing.T) {
	rel := Release{Tag: "v1", Assets: map[string]string{"umbrella-linux-amd64": "x", "SHA256SUMS.txt": "y"}}
	err := apply(context.Background(), rel, "linux", "arm", filepath.Join(t.TempDir(), "u"))
	if err == nil || !strings.Contains(err.Error(), "no build for linux/arm") {
		t.Fatalf("err = %v", err)
	}
}

func TestManualHintOnlyOnWindows(t *testing.T) {
	if h := ManualHint("windows"); !strings.Contains(h, "umbrella-windows-amd64.exe") || !strings.Contains(h, "releases/latest") {
		t.Fatalf("hint = %q", h)
	}
	if ManualHint("linux") != "" || ManualHint("darwin") != "" {
		t.Fatal("no hint outside Windows")
	}
}
