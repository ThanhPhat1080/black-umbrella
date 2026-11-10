// Package clip copies text to the OS clipboard, falling back to OSC 52.
package clip

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/atotto/clipboard"
)

func Copy(s string) error {
	if err := clipboard.WriteAll(s); err == nil {
		return nil
	}
	_, err := fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(s)))
	return err
}
