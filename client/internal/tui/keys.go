package tui

// Key names as reported by bubbletea's KeyMsg.String().
const (
	keyQuit      = "ctrl+c"
	keyNextConv  = "ctrl+n"
	keyPrevConv  = "ctrl+p"
	keyFocus     = "tab"
	keyEsc       = "esc"
	keyEnter     = "enter"
	keyUp        = "up"
	keyDown      = "down"
	keyLeft      = "left"
	keyRight     = "right"
	keyCopyLink  = "y"
	keyKeep      = "a"
	keyOpenThrd  = "r"
	keyOpen      = "o"
	keyCopyMsg   = "c"
	keySelect    = "ctrl+x"
	maxInputRune = 4000
)

const helpLine = "Enter send · Alt+Enter newline · Tab main/thread box · Ctrl+N/P switch · ↑/↓ select · → open thread · ←/Esc close thread · o open link · y copy link · c copy message · a keep · Ctrl+X select mode · /i kaomoji · /h all shortcuts · Esc close"

// helpEntries is the /h cheat sheet: {keys, action}; an empty first column is a blank spacer.
var helpEntries = [][2]string{
	{"Enter", "send"},
	{"Alt+Enter", "new line"},
	{"Ctrl+← / Ctrl+→", "jump a word (also Option+← / →)"},
	{"Home / End", "line start / end (Ctrl+E also = end)"},
	{"Ctrl+W / Ctrl+U", "delete word / delete to line start"},
	{selectAllHelpKey(), "select all in the textbox (copies it)"},
	{"Tab", "switch input / message list (main ⇄ thread box)"},
	{"Ctrl+N / Ctrl+P", "next / previous conversation"},
	{"Esc", "back to the input / close thread"},
	{"", ""},
	{"↑ / ↓", "select message"},
	{"→ or r", "open thread (→ also focuses the reply box)"},
	{"←", "close thread"},
	{"o", "open the message's link"},
	{"y", "copy the message's link"},
	{"c", "copy the message text"},
	{"a", "keep (anchor) your own message"},
	{"", ""},
	{"Mouse drag", "select text and copy it (stays in one pane)"},
	{"Ctrl+X", "select mode: let the terminal select"},
	{"", ""},
	{"/i", "kaomoji picker"},
	{"/whois @user", "show a profile"},
	{"/name <name>", "change your display name"},
	{"/h", "this help"},
	{"/quit, Ctrl+C", "quit"},
}

func selectAllHelpKey() string {
	if ctrlASelectsAll {
		return "Ctrl+A / Ctrl+L"
	}
	return "Ctrl+L / Alt+A"
}
