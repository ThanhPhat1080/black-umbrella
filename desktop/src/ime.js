'use strict';
// Input methods (Vietnamese: IBus + Vikey, Bamboo, Unikey; fcitx; and anything else that composes).
//
// Every input method edits the browser's text field in its own way: some keep a word (or a whole
// sentence) in a preedit and commit it later, some commit letter by letter and then "delete the
// previous N characters, insert the corrected ones". xterm.js only forwards a composition when it
// ends, so with a long-lived preedit the text reaches the client late and in chunks, and a commit
// after a deletion is mis-sent. Here the hidden <textarea> is the source of truth instead: whatever
// the input method does to it, the client is kept identical to it, right away, by sending the
// difference (Backspaces for removed characters, then the added ones). This works the same for
// every engine and both Wayland and X11, because it never looks at *how* the text was changed.
//
// xterm.js keeps handling the keyboard (keys it understands never touch the textarea), and its
// own composition / input handling is kept away from the textarea so nothing is sent twice.

const KEEP = 200; // characters of context kept in the textarea for the input method

const cps = (s) => Array.from(s);

/** What to send so a client that has `sent` ends up with `value`: {del, add}. */
function diff(sent, value) {
  const a = cps(sent);
  const b = cps(value);
  let p = 0;
  while (p < a.length && p < b.length && a[p] === b[p]) p++;
  return { del: a.length - p, add: b.slice(p).join('') };
}

/** `sent` after xterm.js itself sent `data` for a key: typed text is appended, Backspace removes
 * one character, any other control sequence (Enter, arrows, Ctrl+…, pastes) starts over. */
function afterKey(sent, data) {
  if (data === '\x7f') return cps(sent).slice(0, -1).join('');
  if (/[\x00-\x1f\x7f]/.test(data)) return null; // the client's line changed in a way we cannot mirror
  return sent + data;
}

function install(term, send, log) {
  const ta = term.textarea;
  const root = term.element;
  if (!ta || !root) return;
  const desc = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value');
  const get = () => desc.get.call(ta);
  const set = (v) => desc.set.call(ta, v); // puts the caret at the end
  // xterm.js clears or fills the textarea for its own reasons (blur, Enter, Linux middle-click
  // selection); none of that must reach the input method or the client.
  Object.defineProperty(ta, 'value', { configurable: true, get, set() {} });

  let sent = ''; // the text the client has that mirrors the textarea
  let composing = false;
  let timer = null;

  const flush = () => {
    timer = null;
    const value = get();
    const { del, add } = diff(sent, value);
    if (del) send('\x7f'.repeat(del));
    if (add) send(add);
    sent = value;
    if (!composing && cps(sent).length > KEEP) {
      sent = cps(sent).slice(-KEEP / 2).join('');
      set(sent);
    }
    if (log && (del || add)) log(`-> client del=${del} add=${JSON.stringify(add)} value=${JSON.stringify(value)}`);
  };
  // All the DOM events of one key press arrive in the same task; mirror once they are all in.
  const schedule = () => {
    if (timer === null) timer = setTimeout(flush, 0);
  };

  const stop = (e) => e.stopPropagation(); // keeps the event away from xterm's own listeners
  root.addEventListener('compositionstart', (e) => { composing = true; stop(e); }, true);
  root.addEventListener('compositionupdate', stop, true);
  root.addEventListener('compositionend', (e) => { composing = false; stop(e); schedule(); }, true);
  root.addEventListener('input', (e) => { stop(e); schedule(); }, true);
  root.addEventListener('keydown', (e) => {
    if (e.isComposing || e.keyCode === 229 || e.key === 'Process') stop(e); // the input method has this key
  }, true);

  // Keys that xterm.js handled itself (the textarea is untouched): keep the mirror in step.
  term.onData((d) => {
    if (timer !== null) flush(); // never reorder with a pending mirror update
    const next = afterKey(sent, d);
    if (next === null) {
      // Enter, an arrow… the client's line is something we do not know. The textarea is left as it
      // is during a composition (changing it would break the input method): it is just "accounted for".
      sent = composing ? get() : '';
      if (!composing) set('');
    } else {
      sent = next;
      if (!composing) set(sent);
    }
  });

  if (log) {
    for (const n of ['keydown', 'beforeinput', 'input', 'compositionstart', 'compositionupdate', 'compositionend']) {
      ta.addEventListener(n, (e) => log(`${n} key=${e.key ?? ''} code=${e.keyCode ?? ''} type=${e.inputType ?? ''} data=${JSON.stringify(e.data ?? '')} value=${JSON.stringify(get())}`), true);
    }
    term.onData((d) => log(`xterm key -> client ${JSON.stringify(d)} sent=${JSON.stringify(sent)}`));
  }
}

const exported = { diff, afterKey, install };
if (typeof module !== 'undefined' && module.exports) module.exports = exported;
else window.UmbrellaIme = exported;
