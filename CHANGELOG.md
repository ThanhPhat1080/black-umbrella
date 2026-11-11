# Changelog

## v1.2.0 — 2026-11-11
- Client: `/update` (or `umbrella update`) installs the latest release in place, with a SHA-256 check. Your key is untouched; reopen Umbrella afterwards. Versions before v1.2.0 have no updater, so download this one manually once.

## v1.1.0 — 2026-11-11
- Client: Linux support for input methods (IBus, fcitx): the composition popup now appears at the textbox instead of the bottom-left corner of the terminal.
- Client: `/cow [text]` sends an ASCII cow in a cowsay-style speech bubble (centred in the chat; no thread/keep buttons). `/dance` makes a surprise animal dance for everyone (10s cooldown); `/e` hides or shows the dancing cat at the bottom of the sidebar.
- Client: Ctrl+C asks for confirmation before quitting (press it again, or `y`/Enter). `/quit` still quits at once.
- Client: selecting text no longer copies by itself; press Ctrl+Y to copy the selection.
- Client: darker textbox, a blank row between messages and the textbox, "Channel" / "User" sections with a separator and spacing in the sidebar (the user list scrolls), and the copy/keep/thread buttons sit under the text.

## v1.0.0 — 2026-11-10
First stable release. Earlier 0.1.x releases were removed; their history is kept below.

## v0.1.9 — 2026-11-10
- Client: `/buzz` shakes everyone's screen for 1s and shows an animated `⚡ ~~ <name> buzzzz! (ﾉ°Д°)ﾉ ~~ ⚡` banner for 2s (10s cooldown; older clients just see `/buzz`).
- Client: `/iam` shows who you are (display name and username).
- Docs: quick-update commands for macOS, Linux and Windows.

## v0.1.8 — 2026-11-10
- Client: mouse-drag text selection and copy (stays within one pane); message editing.
- Docs: install and update instructions for macOS and Windows.

## v0.1.6 — 2026-11-10
- Client: blank line between messages (chat and thread panels).
- Client: your own messages are right-aligned (and wrap narrower); other users' stay on the left.

## v0.1.5
- Client: clickable links, copy message, select mode.

## v0.1.4
- Client: separate reply textbox in the thread panel; removed target toggle.
- Client: one-line sidebar rows with mini avatars, separator and background highlights.

## v0.1.3
- Client: scroll window instead of bottom-anchored selection; arrow keys open/close threads.
- Docs: Vietnamese guide for running locally and debugging.

## v0.1.2
- Client: italic message bodies and per-user coloured names.

## v0.1.1
- Client: bordered input textbox with click-to-focus.
- Docs: fix macOS install steps.

## v0.1.0
- Initial release: CI builds and publishes release binaries on version tags.
