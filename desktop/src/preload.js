'use strict';
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('umbrella', {
  ready: (cols, rows) => ipcRenderer.send('pty:ready', cols, rows),
  input: (data) => ipcRenderer.send('pty:input', data),
  resize: (cols, rows) => ipcRenderer.send('pty:resize', cols, rows),
  onData: (fn) => ipcRenderer.on('pty:data', (_e, d) => fn(d)),
  hostEvent: (ev) => ipcRenderer.send('host:event', ev),
  writeClipboard: (text) => ipcRenderer.send('clipboard:write', text),
  imeDebug: process.env.UMBRELLA_IME_DEBUG === '1',
  imeLog: (line) => ipcRenderer.send('ime:log', line),
  readClipboard: () => ipcRenderer.invoke('clipboard:read'),
});
