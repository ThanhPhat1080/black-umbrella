package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
