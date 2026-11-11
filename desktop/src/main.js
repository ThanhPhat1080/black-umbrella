'use strict';
const path = require('node:path');
const os = require('node:os');
const { app, BrowserWindow, Menu, Notification, clipboard, ipcMain, net, shell } = require('electron');
const lib = require('./lib');
const { Session } = require('./session');

// Ubuntu 24.04+ restricts unprivileged user namespaces, which breaks Chromium's sandbox for an
// AppImage (it cannot ship a setuid chrome-sandbox). Umbrella is an internal app, so an AppImage
// restarts itself once with --no-sandbox. The switch must be on the real command line: adding it
// at run time (appendSwitch) is too late and breaks shared memory.
const relaunching = process.platform === 'linux' && process.env.APPIMAGE && !process.argv.includes('--no-sandbox');
if (relaunching) {
  app.relaunch({ execPath: process.env.APPIMAGE, args: [...process.argv.slice(1), '--no-sandbox'] });
  app.exit(0);
}

// Let input methods (fcitx5, IBus: Vietnamese typing) work when Chromium runs natively on Wayland.
if (process.platform === 'linux') {
  app.commandLine.appendSwitch('ozone-platform-hint', 'auto');
  app.commandLine.appendSwitch('enable-wayland-ime');
}

const isDev = !app.isPackaged;
const ICON = path.join(__dirname, '..', 'build', 'icon.png');
let win = null;
let session = null;

function send(channel, ...args) {
  if (win && !win.isDestroyed()) win.webContents.send(channel, ...args);
}

function createWindow() {
  win = new BrowserWindow({
    width: 1100,
    height: 720,
    minWidth: 600,
    minHeight: 360,
    title: 'Umbrella',
    icon: ICON,
    backgroundColor: '#14161a',
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      devTools: isDev || process.env.UMBRELLA_DEVTOOLS === '1',
      spellcheck: false,
    },
  });
  // No right-click menu, no navigation away from the app, no popups.
  win.webContents.on('context-menu', (e) => e.preventDefault());
  win.webContents.on('will-navigate', (e) => e.preventDefault());
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:\/\//.test(url)) shell.openExternal(url);
    return { action: 'deny' };
  });
  win.on('page-title-updated', (e) => e.preventDefault()); // the title shows the unread count
  win.on('focus', () => win.flashFrame(false));
  win.on('closed', () => {
    win = null;
  });
  win.loadFile(path.join(__dirname, 'index.html'));
}

function startSession(cols, rows) {
  const bin = lib.resolveBinary({ resourcesPath: process.resourcesPath, appDir: path.join(__dirname, '..') });
  if (!bin) {
    send('pty:data', '\x1b[31mCannot find the umbrella client.\x1b[0m Set UMBRELLA_BIN or run scripts/build-desktop.sh.\r\n');
    return;
  }
  session?.kill();
  session = new Session({
    bin,
    cwd: os.homedir(),
    env: process.env,
    onData: (d) => send('pty:data', d),
    onQuit: () => app.quit(),
  });
  session.start(cols, rows);
}

function showWindow() {
  if (!win) return;
  if (win.isMinimized()) win.restore();
  win.show();
  win.focus();
}

function notify({ title, body, onClick }) {
  if (!Notification.isSupported()) return;
  const n = new Notification({ title, body, icon: ICON, silent: true }); // the client already plays a sound
  n.on('click', onClick || showWindow);
  n.show();
}

/** Something the Go client reported through OSC 777: a notification or the unread count. */
function onHostEvent(ev) {
  if (!win || !ev) return;
  if (ev.k === 'unread') {
    const n = Number(ev.n) || 0;
    win.setTitle(lib.windowTitle(n));
    app.setBadgeCount(n);
    return;
  }
  if (ev.k !== 'notify' || win.isFocused()) return; // when you are looking at it the app already shows it
  const msg = lib.notificationFor(ev);
  if (msg) notify(msg);
  if (ev.kind === 'buzz') win.flashFrame(true);
}

async function checkForUpdate() {
  const current = app.getVersion();
  if (isDev || !lib.isNewer('v999.0.0', current)) return; // dev builds have no release to compare
  try {
    const res = await net.fetch(`https://api.github.com/repos/${lib.REPO}/releases/latest`, {
      headers: { Accept: 'application/vnd.github+json', 'User-Agent': 'umbrella-desktop' },
    });
    if (!res.ok) return;
    const rel = await res.json();
    if (!lib.isNewer(rel.tag_name, current)) return;
    notify({
      title: `Umbrella ${rel.tag_name} is available`,
      body: 'Click to open the download page.',
      onClick: () => shell.openExternal(rel.html_url),
    });
  } catch {
    /* offline: try again next start */
  }
}

if (relaunching) {
  // this process is already exiting; the relaunched one does the work
} else if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', showWindow);
  app.whenReady().then(() => {
    Menu.setApplicationMenu(null); // no menu accelerators stealing Ctrl+C / Ctrl+V / Ctrl+N
    ipcMain.on('pty:ready', (_e, cols, rows) => startSession(cols, rows));
    ipcMain.on('pty:input', (_e, data) => session?.write(String(data)));
    ipcMain.on('pty:resize', (_e, cols, rows) => session?.resize(cols, rows));
    ipcMain.on('host:event', (_e, ev) => onHostEvent(ev));
    ipcMain.on('ime:log', (_e, line) => {
      // UMBRELLA_IME_DEBUG=1: input-method events, for reporting typing problems
      try {
        require('node:fs').appendFileSync(path.join(os.tmpdir(), 'umbrella-ime.log'), `${Date.now()} ${String(line)}\n`);
      } catch {
        /* ignore */
      }
    });
    ipcMain.on('clipboard:write', (_e, text) => clipboard.writeText(String(text)));
    ipcMain.handle('clipboard:read', () => clipboard.readText());
    createWindow();
    if (process.env.UMBRELLA_SMOKE) require('./smoke')(win, process.env.UMBRELLA_SMOKE);
    setTimeout(checkForUpdate, 5000);
  });
  app.on('window-all-closed', () => {
    session?.kill();
    app.quit();
  });
  app.on('before-quit', () => session?.kill());
}
