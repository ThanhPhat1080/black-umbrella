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

const helpLine = "Enter send · Alt+Enter newline · Tab main/thread box · Ctrl+N/P switch · ↑/↓ select · → open thread · ←/Esc close thread · o open link · y copy link · c copy message · a keep · Ctrl+X select mode · /i kaomoji · Esc close"
