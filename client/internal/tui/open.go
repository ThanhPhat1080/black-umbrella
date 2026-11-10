package tui

import (
	"errors"
	"net/url"
	"os/exec"
	"runtime"
	"unicode"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/clip"
)

// Package vars so tests can stub the side effects.
var (
	openURL  = defaultOpen
	copyText = clip.Copy
)

// validURL accepts only plain http(s) URLs without whitespace or control characters.
func validURL(s string) bool {
	if s == "" || s[0] == '-' {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return true
}

// defaultOpen starts the OS URL opener. The URL is a single argv element: no shell is involved.
func defaultOpen(u string) error {
	if !validURL(u) {
		return errors.New("refusing to open a non-http(s) link")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openLink re-validates u right before handing it to the opener.
func (m *Model) openLink(u string) {
	if !validURL(u) {
		m.setStatus("Refusing to open that link.")
		return
	}
	if err := openURL(u); err != nil {
		m.setStatus("Couldn't open link: " + err.Error())
		return
	}
	m.setStatus("Opening " + truncate(u, 60))
}
