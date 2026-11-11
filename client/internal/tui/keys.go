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
	keyCopySel   = "ctrl+y" // copy the current text selection (selecting never copies by itself)
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
	{selectAllHelpKey(), "select all in the textbox (Ctrl+Y copies it)"},
	{keyCopySel, "copy the selected text (selecting never copies by itself)"},
	{"Tab", "cycle input → message list → sidebar (thread box when open)"},
	{"Sidebar: ↑/↓, →", "pick a conversation, → or Enter opens it (Esc back)"},
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
	{"@", "pick someone to mention (↑/↓, Tab/Enter, Esc)"},
	{"Mouse drag", "select text and copy it (stays in one pane)"},
	{"Ctrl+X", "select mode: let the terminal select"},
	{"", ""},
	{"/room <name> [@u..]", "create a private room (and add people)"},
	{"/invite @user", "add people to the open room"},
	{"/members", "who is in the open room"},
	{"/roomname <name>", "rename the room (owner)"},
	{"/kick @user", "remove someone (owner)"},
	{"/leave", "leave the open room"},
	{"/closeroom", "delete the room for everyone (owner)"},
	{"", ""},
	{"/update", "install the latest version"},
	{"/buzz", "shake everyone's screen"},
	{"/dance", "a surprise animal dances for everyone"},
	{"/cow [text]", "send an ASCII cow (saying text)"},
	{"/cat [text]", "send an ASCII cat (saying text)"},
	{"/rabbit [text]", "send an ASCII rabbit"},
	{"/dog [text]", "send an ASCII dog"},
	{"/duck [text]", "send an ASCII duck"},
	{"/fish", "a school of fish swims across the chat"},
	{"/e", "hide / show the sidebar pet"},
	{"/i", "kaomoji picker"},
	{"/iam", "show who you are"},
	{"/whois @user", "show a profile"},
	{"/name <name>", "change your display name"},
	{"/h", "this help"},
	{"/quit", "quit now"},
	{"Ctrl+C", "quit (asks to confirm; press again or y/Enter)"},
}

func selectAllHelpKey() string {
	if ctrlASelectsAll {
		return "Ctrl+A / Ctrl+L"
	}
	return "Ctrl+L / Alt+A"
}
