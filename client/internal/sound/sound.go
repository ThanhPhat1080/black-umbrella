// Package sound plays a short system alert sound, falling back to the terminal bell.
package sound

import (
	"os"
	"os/exec"
	"runtime"
)

// Buzz plays a short alert. It never blocks: the player is started and reaped in the
// background. When no system player is available it rings the terminal bell instead.
func Buzz() { play("Funk", "message") }

// Mention plays a softer alert for when someone mentions you.
func Mention() { play("Ping", "message") }

func play(mac, linux string) {
	name, args := command(mac, linux)
	if name != "" {
		if path, err := exec.LookPath(name); err == nil {
			cmd := exec.Command(path, args...)
			if cmd.Start() == nil {
				go func() { _ = cmd.Wait() }()
				return
			}
		}
	}
	_, _ = os.Stdout.WriteString("\a") // zero-width, one write(2): cannot corrupt a frame
}

func command(mac, linux string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "afplay", []string{"/System/Library/Sounds/" + mac + ".aiff"}
	case "linux":
		for _, f := range []string{
			"/usr/share/sounds/freedesktop/stereo/" + linux + ".oga",
			"/usr/share/sounds/freedesktop/stereo/bell.oga",
		} {
			if _, err := os.Stat(f); err == nil {
				return "paplay", []string{f}
			}
		}
	case "windows":
		return "powershell", []string{"-NoProfile", "-Command", "[System.Media.SystemSounds]::Exclamation.Play()"}
	}
	return "", nil
}
