// Package update replaces the running umbrella binary with the latest GitHub release.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub repository whose releases are installed.
const Repo = "ThanhPhat1080/black-umbrella"

var apiBase = "https://api.github.com"

// Release is the latest published release.
type Release struct {
	Tag    string
	Assets map[string]string // asset name -> download URL
}

func assetName(goos, goarch string) string {
	n := "umbrella-" + goos + "-" + goarch
	if goos == "windows" {
		n += ".exe"
	}
	return n
}

func parse(v string) (p [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return p, false
	}
	for i, s := range parts {
		n, err := strconv.Atoi(s)
		if err != nil {
			return p, false
		}
		p[i] = n
	}
	return p, true
}

// Newer reports whether latest is a newer version than current. An unparseable
// current version (a dev build or an old binary) counts as out of date.
func Newer(latest, current string) bool {
	l, ok := parse(latest)
	if !ok {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return true
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "umbrella-updater")
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// Latest fetches the newest release.
func Latest(ctx context.Context) (Release, error) {
	resp, err := get(ctx, apiBase+"/repos/"+Repo+"/releases/latest")
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	var raw struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&raw); err != nil {
		return Release{}, err
	}
	r := Release{Tag: raw.Tag, Assets: map[string]string{}}
	for _, a := range raw.Assets {
		r.Assets[a.Name] = a.URL
	}
	return r, nil
}

func checksumFor(ctx context.Context, url, name string) (string, error) {
	resp, err := get(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum listed for %s", name)
}

func apply(ctx context.Context, rel Release, goos, goarch, exe string) error {
	name := assetName(goos, goarch)
	binURL, ok := rel.Assets[name]
	if !ok {
		return fmt.Errorf("release %s has no build for %s/%s", rel.Tag, goos, goarch)
	}
	sumURL, ok := rel.Assets["SHA256SUMS.txt"]
	if !ok {
		return errors.New("release has no SHA256SUMS.txt; refusing to install")
	}
	want, err := checksumFor(ctx, sumURL, name)
	if err != nil {
		return err
	}
	resp, err := get(ctx, binURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".umbrella-update-*")
	if err != nil {
		return fmt.Errorf("can't write next to %s (try sudo?): %w", exe, err)
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return errors.New("checksum mismatch; download discarded")
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if goos == "windows" { // a running .exe can be renamed but not overwritten
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), exe); err != nil {
			os.Rename(old, exe)
			return err
		}
		return nil
	}
	return os.Rename(tmp.Name(), exe)
}

// Run updates the running binary if a newer release exists. It returns the
// installed tag and whether anything changed.
func Run(ctx context.Context, current string) (tag string, updated bool, err error) {
	rel, err := Latest(ctx)
	if err != nil {
		return "", false, fmt.Errorf("can't check for updates: %w", err)
	}
	if !Newer(rel.Tag, current) {
		return rel.Tag, false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	if err := apply(ctx, rel, runtime.GOOS, runtime.GOARCH, exe); err != nil {
		return "", false, err
	}
	return rel.Tag, true, nil
}
