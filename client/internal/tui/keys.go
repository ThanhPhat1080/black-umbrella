package tui

// Key names as reported by bubbletea's KeyMsg.String().
const (
	keyQuit      = "ctrl+c"
	keyNextConv  = "ctrl+n"
	keyPrevConv  = "ctrl+p"
	keyFocus     = "tab"
	keyTarget    = "ctrl+t"
	keyEsc       = "esc"
	keyEnter     = "enter"
	keyUp        = "up"
	keyDown      = "down"
	keyLeft      = "left"
	keyRight     = "right"
	keyCopyLink  = "y"
	keyKeep      = "a"
	keyOpenThrd  = "r"
	maxInputRune = 4000
)

const helpLine = "Enter send · Alt+Enter newline · Tab focus · Ctrl+N/P switch · ↑/↓ select · → open thread · ←/Esc close thread · y copy link · a keep · /i kaomoji · Ctrl+T thread/main · Esc close"
