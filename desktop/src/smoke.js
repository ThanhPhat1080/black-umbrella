'use strict';
// Developer smoke test, not used unless UMBRELLA_SMOKE is set:
//   UMBRELLA_SMOKE=/path/last.png UMBRELLA_SMOKE_STEPS='[{"wait":3000},{"type":"123456\r"}]' electron .
// Steps: {"eval":"js"} (prints the result) {"wait":ms} {"type":"text"} (\r = Enter) {"ctrl":"A"} {"click":[x,y]} {"shot":"file.png"}.
// At the end it saves the final screenshot, prints the window title and the clipboard as JSON and quits.
const fs = require('node:fs');
const { app, clipboard } = require('electron');

const DEFAULT_STEPS = [
  { wait: 3000 },
  { clip: 'DÁN từ clipboard ạ' },
  { ctrl: 'V' }, // paste
  { ctrl: 'C' }, // arrives as ^C
  { ctrl: 'A' }, // arrives as ^A
  { wait: 2500 },
];

module.exports = function smoke(win, shotPath) {
  const wc = win.webContents;
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const shot = async (file) => fs.writeFileSync(file, (await wc.capturePage()).toPNG());
  (async () => {
    const steps = process.env.UMBRELLA_SMOKE_STEPS ? JSON.parse(process.env.UMBRELLA_SMOKE_STEPS) : DEFAULT_STEPS;
    let osc52 = '';
    win.show();
    win.focus(); // Wayland only lets the focused window use the clipboard
    for (const s of steps) {
      if (s.wait) await sleep(s.wait);
      else if (s.clip !== undefined) {
        osc52 = clipboard.readText(); // what the client copied through OSC 52 so far
        clipboard.writeText(s.clip);
      } else if (s.type !== undefined) {
        for (const ch of s.type) {
          if (ch === '\r') {
            wc.sendInputEvent({ type: 'keyDown', keyCode: 'Return' });
            wc.sendInputEvent({ type: 'keyUp', keyCode: 'Return' });
          } else {
            wc.sendInputEvent({ type: 'char', keyCode: ch });
          }
          await sleep(15);
        }
      } else if (s.keys !== undefined) {
        // like a real keyboard: key down/up only (xterm handles the key and cancels the text input)
        for (const ch of s.keys) {
          wc.sendInputEvent({ type: 'keyDown', keyCode: ch });
          wc.sendInputEvent({ type: 'keyUp', keyCode: ch });
          await sleep(40);
        }
      } else if (s.ctrl) {
        wc.sendInputEvent({ type: 'keyDown', keyCode: s.ctrl, modifiers: ['control'] });
        wc.sendInputEvent({ type: 'keyUp', keyCode: s.ctrl, modifiers: ['control'] });
      } else if (s.click) {
        const [x, y] = s.click;
        wc.sendInputEvent({ type: 'mouseDown', x, y, button: 'left', clickCount: 1 });
        wc.sendInputEvent({ type: 'mouseUp', x, y, button: 'left', clickCount: 1 });
      } else if (s.xtype !== undefined) {
        // real key events through XTEST (X11/XWayland only), sent only while this window has focus
        if (win.isFocused() || s.force) await new Promise((r) => require('node:child_process').execFile('python3', [s.script, s.xtype], r));
        else console.log('XTYPE skipped: window not focused');
      } else if (s.shot) await shot(s.shot);
      else if (s.eval) console.log('EVAL ' + JSON.stringify(await wc.executeJavaScript(s.eval)));
    }
    await shot(shotPath);
    console.log('SMOKE ' + JSON.stringify({ focused: win.isFocused(), title: win.getTitle(), osc52, clipboard: clipboard.readText() }));
    app.quit();
  })();
};
