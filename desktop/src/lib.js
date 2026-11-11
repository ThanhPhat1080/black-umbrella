'use strict';
// Pure helpers used by the main process (kept free of Electron so `node --test` can run them).
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const REPO = 'ThanhPhat1080/black-umbrella';

/** Directory that holds key.json: $UMBRELLA_HOME, or ~/.umbrella (same rule as the Go client). */
function configDir(env = process.env, home = os.homedir()) {
  return env.UMBRELLA_HOME || path.join(home, '.umbrella');
}

function hasKey(env = process.env, home = os.homedir()) {
  return fs.existsSync(path.join(configDir(env, home), 'key.json'));
}

/** Finds the bundled `umbrella` client: $UMBRELLA_BIN, resources/bin (packaged), bin/ or dist/ (dev). */
function resolveBinary({ resourcesPath, appDir, env = process.env, exists = fs.existsSync }) {
  const candidates = [];
  if (env.UMBRELLA_BIN) candidates.push(env.UMBRELLA_BIN);
  if (resourcesPath) candidates.push(path.join(resourcesPath, 'bin', 'umbrella'));
  candidates.push(
    path.join(appDir, 'bin', 'umbrella'),
    path.join(appDir, '..', 'dist', 'umbrella-linux-amd64'),
  );
  return candidates.find((c) => exists(c)) || null;
}

/** Decodes the payload of an OSC 777 sequence from the Go client ("umbrella;<base64 json>"). */
function parseHostEvent(data) {
  const prefix = 'umbrella;';
  if (typeof data !== 'string' || !data.startsWith(prefix)) return null;
  try {
    const json = Buffer.from(data.slice(prefix.length), 'base64').toString('utf8');
    const ev = JSON.parse(json);
    return ev && typeof ev.k === 'string' ? ev : null;
  } catch {
    return null;
  }
}

/** Decodes an OSC 52 clipboard write ("c;<base64>"); null for queries ("?") or garbage. */
function parseClipboardWrite(data) {
  const i = typeof data === 'string' ? data.indexOf(';') : -1;
  if (i < 0) return null;
  const b64 = data.slice(i + 1);
  if (!b64 || b64 === '?') return null;
  try {
    return Buffer.from(b64, 'base64').toString('utf8');
  } catch {
    return null;
  }
}

function parseVersion(v) {
  const m = /^v?(\d+)\.(\d+)\.(\d+)/.exec(String(v));
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null;
}

/** True when `latest` is a newer release than `current` ("v1.7.0" vs "1.6.1"); false if unparsable. */
function isNewer(latest, current) {
  const a = parseVersion(latest);
  const b = parseVersion(current);
  if (!a || !b) return false;
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] > b[i];
  }
  return false;
}

/** The title and body of the desktop notification for a Go `notify` event, or null for none. */
function notificationFor(ev) {
  if (!ev || ev.k !== 'notify') return null;
  const from = ev.from || 'Someone';
  switch (ev.kind) {
    case 'buzz':
      return { title: `⚡ ${from} buzzed you!`, body: '' };
    case 'mention':
      return { title: `${from} mentioned you`, body: ev.text || '' };
    case 'dm':
      return { title: from, body: ev.text || '' };
    default:
      return null;
  }
}

function windowTitle(unread) {
  return unread > 0 ? `(${unread}) Umbrella` : 'Umbrella';
}

module.exports = {
  REPO,
  configDir,
  hasKey,
  resolveBinary,
  parseHostEvent,
  parseClipboardWrite,
  isNewer,
  notificationFor,
  windowTitle,
};
