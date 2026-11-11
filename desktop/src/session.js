'use strict';
// Runs the Go client in a pseudo-terminal: `umbrella register` first when there is no key yet,
// then `umbrella` itself. After a failed run (wrong PIN, cancelled sign-up) Enter starts over.
const pty = require('@lydell/node-pty');
const { hasKey } = require('./lib');

const AGAIN = '\r\n\x1b[2mPress Enter to try again, or close the window to quit.\x1b[0m\r\n';

class Session {
  /**
   * @param {{bin: string, cwd: string, env: object, onData: (s: string) => void, onQuit: () => void}} o
   */
  constructor(o) {
    this.o = o;
    this.cols = 100;
    this.rows = 30;
    this.proc = null;
    this.waiting = false; // true while the "press Enter" prompt is showing
  }

  start(cols, rows) {
    if (cols) this.cols = cols;
    if (rows) this.rows = rows;
    this.launch();
  }

  launch() {
    this.waiting = false;
    const registering = !hasKey(this.o.env);
    const proc = pty.spawn(this.o.bin, registering ? ['register'] : [], {
      name: 'xterm-256color',
      cols: this.cols,
      rows: this.rows,
      cwd: this.o.cwd,
      env: { ...this.o.env, TERM: 'xterm-256color', COLORTERM: 'truecolor', UMBRELLA_HOST: 'electron' },
    });
    this.proc = proc;
    proc.onData((d) => this.o.onData(d));
    proc.onExit(({ exitCode, signal }) => {
      if (this.proc !== proc) return; // a stale process
      this.proc = null;
      const ok = exitCode === 0 && !signal; // a process killed by Ctrl+C reports exit code 0 plus a signal
      if (registering && ok) return this.launch(); // signed up: go straight to the chat
      if (ok) return this.o.onQuit(); // /quit
      this.waiting = true;
      this.o.onData(AGAIN);
    });
  }

  write(data) {
    if (this.waiting) {
      if (data.includes('\r')) this.launch();
      return;
    }
    if (this.proc) this.proc.write(data);
  }

  resize(cols, rows) {
    this.cols = cols;
    this.rows = rows;
    if (this.proc) {
      try {
        this.proc.resize(cols, rows);
      } catch {
        /* the process just exited */
      }
    }
  }

  kill() {
    const p = this.proc;
    this.proc = null;
    if (p) {
      try {
        p.kill();
      } catch {
        /* already gone */
      }
    }
  }
}

module.exports = { Session };
