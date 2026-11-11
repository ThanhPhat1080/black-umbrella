'use strict';
/* global Terminal, FitAddon, Unicode11Addon, WebglAddon, UmbrellaIme */
const api = window.umbrella;

const FONT = '"JetBrains Mono", "DejaVu Sans Mono", monospace, "Noto Color Emoji", "Apple Color Emoji", "Segoe UI Emoji"';
const SIZE_KEY = 'umbrella.fontSize';
const MIN_SIZE = 9;
const MAX_SIZE = 32;

function savedSize() {
  try {
    const n = Number(localStorage.getItem(SIZE_KEY));
    return n >= MIN_SIZE && n <= MAX_SIZE ? n : 15;
  } catch {
    return 15;
  }
}

// --- smooth window shake for /buzz and mentions (the Go client's own column shake is off) ----
// Offsets (in "cells") mirror the terminal client: a decaying left/right wobble.
const BUZZ_OFFSETS = [3, -3, 3, -3, 2, -2, 2, -2, 2, -2, 1, -1, 1, -1, 1, -1, 1, 0, 0, 0];
const NUDGE_OFFSETS = [1, -1, 1, -1, 1, 0, 0, 0, 0];
const BUZZ_SHAKE_MS = 1000;
const BUZZ_GAP_MS = 6000; // shakes again every few seconds ...
const BUZZ_TOTAL_MS = 10000; // ... for ten seconds
const NUDGE_MS = 450;

function shake(el, offsets, durationMs, pxPerCell) {
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  const frames = offsets.map((o) => ({ transform: `translateX(${o * pxPerCell}px)` }));
  el.animate(frames, { duration: durationMs, easing: 'linear' });
}

function buzz(el) {
  for (let t = 0; t < BUZZ_TOTAL_MS; t += BUZZ_GAP_MS) {
    setTimeout(() => shake(el, BUZZ_OFFSETS, BUZZ_SHAKE_MS, 7), t);
  }
}

async function main() {
  // Cell sizes are measured when the terminal opens, so the fonts must be ready first.
  const size = savedSize();
  try {
    await Promise.all([
      document.fonts.load(`${size}px "JetBrains Mono"`),
      document.fonts.load(`bold ${size}px "JetBrains Mono"`),
      document.fonts.load(`${size}px "JetBrains Mono"`, 'ạ ế ữ'),
    ]);
  } catch {
    /* fall back to the system monospace font */
  }

  const term = new Terminal({
    fontFamily: FONT,
    fontSize: size,
    lineHeight: 1,
    cursorBlink: true,
    cursorStyle: 'block',
    allowProposedApi: true, // the unicode11 addon needs it
    scrollback: 0, // the TUI owns the whole screen
    rightClickSelectsWord: false,
    theme: { background: '#14161a', foreground: '#e6e6e6', cursor: '#e6e6e6', selectionBackground: '#3a4660' },
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.loadAddon(new Unicode11Addon.Unicode11Addon());
  term.unicode.activeVersion = '11'; // emoji and CJK are two cells wide, like the Go client assumes
  term.open(document.getElementById('term'));
  // The WebGL renderer draws box-drawing characters itself, so the borders have no gaps. If the
  // GPU is unavailable the default DOM renderer stays.
  try {
    const gl = new WebglAddon.WebglAddon();
    gl.onContextLoss(() => gl.dispose());
    term.loadAddon(gl);
  } catch {
    /* keep the DOM renderer */
  }
  fit.fit();

  // --- input and output --------------------------------------------------------------------
  term.onData((d) => api.input(d));
  UmbrellaIme.install(term, (d) => api.input(d), api.imeDebug ? api.imeLog : null); // keeps the client identical to the IME's text field
  api.onData((d) => term.write(d));
  term.onResize(({ cols, rows }) => api.resize(cols, rows));

  // --- web-style keyboard -------------------------------------------------------------------
  // Ctrl+C / Ctrl+X / Ctrl+A reach the client as control characters and it copies, cuts and
  // selects (its mouse selection lives in the client). Ctrl+V pastes the system clipboard.
  const paste = async () => {
    const text = await api.readClipboard();
    if (text) term.paste(text);
  };
  const zoom = (delta) => {
    const next = delta === 0 ? 15 : Math.min(MAX_SIZE, Math.max(MIN_SIZE, term.options.fontSize + delta));
    term.options.fontSize = next;
    try {
      localStorage.setItem(SIZE_KEY, String(next));
    } catch {
      /* private mode */
    }
    fit.fit();
  };
  term.attachCustomKeyEventHandler((ev) => {
    if (ev.type !== 'keydown' || !ev.ctrlKey || ev.altKey || ev.metaKey) return true;
    const k = ev.key.toLowerCase();
    if (k === 'v') {
      ev.preventDefault();
      paste();
      return false;
    }
    if (k === '=' || k === '+') return zoom(1), ev.preventDefault(), false;
    if (k === '-' || k === '_') return zoom(-1), ev.preventDefault(), false;
    if (k === '0') return zoom(0), ev.preventDefault(), false;
    return true;
  });
  // Pastes that do not come from the keyboard (the browser paste event) reach xterm directly.

  // --- no context menu ----------------------------------------------------------------------
  window.addEventListener('contextmenu', (e) => e.preventDefault());

  // --- escape sequences from the Go client --------------------------------------------------
  // OSC 52: copy to the system clipboard.
  term.parser.registerOscHandler(52, (data) => {
    const i = data.indexOf(';');
    const b64 = i < 0 ? '' : data.slice(i + 1);
    if (b64 && b64 !== '?') {
      try {
        const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
        api.writeClipboard(new TextDecoder().decode(bytes));
      } catch {
        /* malformed payload */
      }
    }
    return true;
  });
  // OSC 777: notifications, buzz and the unread count.
  const shaker = document.getElementById('shaker');
  term.parser.registerOscHandler(777, (data) => {
    if (!data.startsWith('umbrella;')) return false;
    let ev;
    try {
      const bytes = Uint8Array.from(atob(data.slice('umbrella;'.length)), (c) => c.charCodeAt(0));
      ev = JSON.parse(new TextDecoder().decode(bytes));
    } catch {
      return true;
    }
    if (ev.k === 'notify' && ev.active && document.hasFocus()) {
      if (ev.kind === 'buzz') buzz(shaker);
      else if (ev.kind === 'mention') shake(shaker, NUDGE_OFFSETS, NUDGE_MS, 4);
    }
    api.hostEvent(ev); // the main process shows the notification and the unread count
    return true;
  });

  // --- size ----------------------------------------------------------------------------------
  new ResizeObserver(() => fit.fit()).observe(document.getElementById('term'));
  document.fonts.ready.then(() => fit.fit());
  term.focus();
  window.addEventListener('focus', () => term.focus());

  api.ready(term.cols, term.rows);
}

main();
