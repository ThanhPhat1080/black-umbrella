// Package host talks to the program that embeds the client. When Umbrella runs inside the
// Electron desktop app (UMBRELLA_HOST=electron) the app owns the clipboard, notifications and
// the window, and the client signals it with private OSC 777 escape sequences.
package host

import (
	"encoding/base64"
	"encoding/json"
	"os"
)

// Electron reports whether the client runs inside the desktop app.
func Electron() bool { return os.Getenv("UMBRELLA_HOST") == "electron" }

// Emit sends v (JSON) to the desktop app as ESC ] 777 ; umbrella ; <base64> BEL. It is a
// zero-width sequence written in a single write(2), so it cannot split a Bubble Tea frame.
func Emit(v any) {
	_, _ = os.Stdout.WriteString(Sequence(v))
}

// Sequence returns the escape sequence Emit writes ("" if v cannot be encoded).
func Sequence(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return "\x1b]777;umbrella;" + base64.StdEncoding.EncodeToString(b) + "\x07"
}
