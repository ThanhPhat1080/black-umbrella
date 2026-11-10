// Package clip copies text to the OS clipboard, falling back to OSC 52.
package clip

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/atotto/clipboard"
)

// Copy puts s on the clipboard. The OSC 52 fallback is a zero-width sequence emitted in a
// single write(2), so it cannot interleave with (or split) one of Bubble Tea's frame writes.
func Copy(s string) error {
	if err := clipboard.WriteAll(s); err == nil {
		return nil
	}
	_, err := os.Stdout.WriteString(fmt.Sprintf("\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(s))))
	return err
}
