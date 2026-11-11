# Changelog

## v1.5.2 — 2026-11-11
- Client: pressing `↓` (or `j`) on the last message of the list, or the last reply of a thread, now moves focus down into the matching textbox.

## v1.5.1 — 2026-11-11
- Client: picking someone in the `@` mention popup now shows their display name in the textbox (`@Phát 🌂`); it is turned back into `@username` when the message is sent, so the wire format is unchanged.

## v1.5.0 — 2026-11-11
- Client: the conversation header shows one row of people; when they do not all fit, a `+N · Show more` button opens a members modal with a scrollable list, an `[X]` button, Esc to close, `↑/↓` to select and Enter or a click to open that person's chat. `/members` now works in every conversation.
- Client: Tab now also reaches the left sidebar (input → messages → sidebar). There `↑/↓` pick a conversation and `→` or Enter opens it; the focused sidebar gets a heavy border and a tinted background. A long user list now scrolls with the cursor and the mouse wheel.

## v1.4.1 — 2026-11-11
- Client: every conversation header (Hell room, private rooms, direct chats) now has the room-style layout: title with member and online counts, the list of people (green dot = online, click a name to chat), and a divider, on a slightly tinted background.

## v1.4.0 — 2026-11-11
- Client: `@mentions` show the person's display name (`● @Phát 🌂`) instead of the username.
- Client: a `/buzz` plays a system alert sound; being `@mentioned` plays a softer one (macOS system sounds, Windows system sound, freedesktop sounds on Linux, terminal bell as fallback).
- Client: a buzz no longer shows the thread-reply button and cannot be opened as a thread.

## v1.3.2 — 2026-11-11
- Client: on Windows, a failed `/update` now says so clearly and points to the download page; permission errors there suggest running PowerShell as administrator instead of `sudo`. Linux `/update` (including through `sudo` and symlinks) was checked in a container.

## v1.3.1 — 2026-11-11
- Client: `/update` no longer fails when Umbrella lives in a folder only an administrator can write to (e.g. `/usr/local/bin` on macOS): it hands the terminal to `sudo umbrella update`, asks for your password, then returns to the chat.

## v1.3.0 — 2026-11-11
- Client: `@mentions` of known users show as a presence dot plus the name in that user's colour (`● @bob`). Click a mention, a name in a message header or a name in the room's member list to open the chat with that person. The mouse pointer turns into a hand over clickable names (terminals that support OSC 22: kitty, WezTerm, foot, Ghostty, xterm) and the item under the mouse is highlighted everywhere.
- Client: drag the sidebar's right edge with the mouse to make it narrower or wider (16-50 columns).
- Client: being `@mentioned` (in Hell or a room) now also nudges your window: a gentle half-second wiggle, much softer than `/buzz`.
- Client: typing `@` opens a list of people to mention (everyone in Hell, the members in a room, the other person in a DM). Up/Down to choose, Tab or Enter to insert, Esc to close it; deleting the `@` closes it too.
- Rooms: private group chats. `/room <name> [@user …]` creates one (people you list are added straight away), `/invite @user`, `/members`, `/leave`; the owner can `/roomname`, `/kick @user` and `/closeroom` (deletes everything). Up to 20 people per room and 10 rooms per person. Only members can see a room exists; messages are end-to-end encrypted for the current members, so newcomers don't see earlier messages and people who leave or are kicked lose them. Needs the new client: older versions don't see rooms (type `/update`).

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
