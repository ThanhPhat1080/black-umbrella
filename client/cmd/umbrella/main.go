// Command umbrella is the Umbrella terminal chat client.
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/api"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/store"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/tui"
)

// DefaultServer is baked in at build time: -ldflags "-X main.DefaultServer=https://…".
var DefaultServer = "http://localhost:8787"

const usage = `Umbrella - end-to-end encrypted terminal chat

Usage:
  umbrella [--server URL]                       open the chat
  umbrella register [--force] [--server URL]    create your account (needs an invite code)
  umbrella export-key <file>                    back up your encrypted key file
  umbrella admin invite [--server URL]          create an invite code
  umbrella admin reset <user> [--server URL]    let a user re-register with a new key
  umbrella admin unlock <user> [--server URL]   clear a user's PIN lockout

Environment:
  UMBRELLA_ADMIN_TOKEN   admin token for 'umbrella admin' (otherwise prompted)
  UMBRELLA_HOME          config directory (default ~/.umbrella, %APPDATA%\umbrella on Windows)
`

var (
	usernameRe = regexp.MustCompile(`^[a-z0-9_]{3,20}$`)
	pinRe      = regexp.MustCompile(`^[0-9]{6}$`)
)

type prompter struct{ in *bufio.Reader }

func (p *prompter) line(label string) string {
	fmt.Print(label)
	s, _ := p.in.ReadString('\n')
	return strings.TrimSpace(s)
}

func (p *prompter) secret(label string) string {
	fmt.Print(label)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, _ := term.ReadPassword(fd)
		fmt.Println()
		return strings.TrimSpace(string(b))
	}
	s, _ := p.in.ReadString('\n')
	fmt.Println()
	return strings.TrimSpace(s)
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

type opts struct {
	server string
	force  bool
	args   []string
}

func parseArgs(argv []string) (opts, error) {
	var o opts
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--help" || a == "-h" || a == "help":
			fmt.Print(usage)
			os.Exit(0)
		case a == "--force":
			o.force = true
		case a == "--server" || a == "-server":
			if i+1 >= len(argv) {
				return o, errors.New("--server needs a URL")
			}
			i++
			o.server = argv[i]
		case strings.HasPrefix(a, "--server="):
			o.server = strings.TrimPrefix(a, "--server=")
		case strings.HasPrefix(a, "-") && a != "-":
			return o, fmt.Errorf("unknown flag %s", a)
		default:
			o.args = append(o.args, a)
		}
	}
	return o, nil
}

func keyPath() string {
	d, err := store.Dir()
	if err != nil {
		fail("Can't find a config directory: %v", err)
	}
	return filepath.Join(d, "key.json")
}

func main() {
	o, err := parseArgs(os.Args[1:])
	if err != nil {
		fail("%v\n\n%s", err, usage)
	}
	server := o.server
	if server == "" {
		server = DefaultServer
	}
	p := &prompter{in: bufio.NewReader(os.Stdin)}
	cmd := ""
	if len(o.args) > 0 {
		cmd = o.args[0]
	}
	switch cmd {
	case "":
		runChat(p, o.server)
	case "register":
		runRegister(p, server, o.force)
	case "export-key":
		if len(o.args) != 2 {
			fail("Usage: umbrella export-key <file>")
		}
		if err := store.ExportKey(keyPath(), o.args[1]); err != nil {
			fail("Export failed: %v", err)
		}
		fmt.Println("Encrypted key written to", o.args[1])
	case "admin":
		runAdmin(p, server, o.args[1:])
	default:
		fail("Unknown command %q\n\n%s", cmd, usage)
	}
}

func apiError(err error) string {
	var ae *api.APIError
	if errors.As(err, &ae) {
		switch ae.Code {
		case "locked":
			return "Account locked until " + time.UnixMilli(ae.Until).Local().Format("15:04") + "."
		case "bad_credentials":
			return "Wrong PIN."
		case "invalid_invite":
			return "That invite code is invalid or already used."
		case "username_taken":
			return "That username is taken."
		case "invalid_username", "invalid_display_name", "invalid_pin", "invalid_public_key":
			return "The server rejected your details (" + ae.Code + ")."
		case "unauthorized":
			return "Not authorised (check the admin token)."
		case "user_not_found":
			return "No such user."
		}
		return "Server error: " + ae.Code
	}
	return "Can't reach the server: " + err.Error()
}

func runChat(p *prompter, serverFlag string) {
	path := keyPath()
	if _, err := os.Stat(path); err != nil {
		fail("No key found at %s.\nRun `umbrella register` first.", path)
	}
	pin := p.secret("PIN: ")
	kf, kp, err := store.LoadKey(path, pin)
	if errors.Is(err, store.ErrWrongPIN) {
		fail("Wrong PIN.") // decided locally: no network call
	} else if err != nil {
		fail("Can't read key file: %v", err)
	}
	server := serverFlag
	if server == "" {
		server = kf.Server
	}
	if server == "" {
		server = DefaultServer
	}
	c := &api.Client{BaseURL: server}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	connect := func(ctx context.Context) (*api.Conn, error) {
		tok, err := c.Login(ctx, kf.Username, pin)
		if err != nil {
			return nil, err
		}
		return api.Dial(ctx, server, tok)
	}
	conn, err := connect(ctx)
	if err != nil {
		fail("%s", apiError(err))
	}
	st := chat.New(kf.Username, kp)
	if err := tui.Run(ctx, conn, st, kp, connect); err != nil && !errors.Is(err, context.Canceled) {
		fail("%v", err)
	}
}

func runRegister(p *prompter, server string, force bool) {
	path := keyPath()
	if _, err := os.Stat(path); err == nil && !force {
		fail("A key file already exists at %s.\nUse --force to replace it (you will lose access to the old account unless you have a backup).", path)
	}
	invite := p.line("Invite code: ")
	username := p.line("Username (a-z, 0-9, _; 3-20): ")
	if !usernameRe.MatchString(username) {
		fail("Username must be 3-20 characters of a-z, 0-9 or _.")
	}
	display := strings.TrimSpace(p.line("Display name: "))
	if n := utf8.RuneCountInString(display); n < 1 || n > 32 {
		fail("Display name must be 1-32 characters.")
	}
	pin := p.secret("PIN (6 digits): ")
	if !pinRe.MatchString(pin) {
		fail("PIN must be exactly 6 digits.")
	}
	if p.secret("Repeat PIN: ") != pin {
		fail("PINs do not match.")
	}
	kp, err := e2e.GenerateKeyPair()
	if err != nil {
		fail("Key generation failed: %v", err)
	}
	pending := path + ".pending"
	if err := store.SaveKey(pending, username, server, pin, kp); err != nil {
		fail("Can't write key file: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := &api.Client{BaseURL: server}
	_, err = c.Register(ctx, api.RegisterReq{Invite: invite, Username: username, DisplayName: display, Pin: pin,
		PublicKey: base64.StdEncoding.EncodeToString(kp.Public[:])})
	if err != nil {
		_ = os.Remove(pending)
		fail("%s", apiError(err))
	}
	if err := os.Rename(pending, path); err != nil {
		fail("Registered, but couldn't move the key file into place: %v (it is at %s)", err, pending)
	}
	fmt.Printf("Welcome, %s! Your key is saved at %s.\nBack it up with `umbrella export-key <file>`. Run `umbrella` to chat.\n", display, path)
}

func runAdmin(p *prompter, server string, args []string) {
	if len(args) == 0 {
		fail("Usage: umbrella admin invite | reset <user> | unlock <user>")
	}
	token := os.Getenv("UMBRELLA_ADMIN_TOKEN")
	if token == "" {
		token = p.secret("Admin token: ")
	}
	c := &api.Client{BaseURL: server}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch args[0] {
	case "invite":
		code, err := c.AdminInvite(ctx, token)
		if err != nil {
			fail("%s", apiError(err))
		}
		fmt.Println(code)
	case "reset", "unlock":
		if len(args) != 2 {
			fail("Usage: umbrella admin %s <user>", args[0])
		}
		if args[0] == "reset" {
			code, err := c.AdminReset(ctx, token, args[1])
			if err != nil {
				fail("%s", apiError(err))
			}
			fmt.Println(code)
		} else if err := c.AdminUnlock(ctx, token, args[1]); err != nil {
			fail("%s", apiError(err))
		} else {
			fmt.Println("Unlocked", args[1])
		}
	default:
		fail("Unknown admin command %q", args[0])
	}
}
