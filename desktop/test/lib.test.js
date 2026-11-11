'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const lib = require('../src/lib');

test('configDir follows UMBRELLA_HOME, then ~/.umbrella', () => {
  assert.equal(lib.configDir({ UMBRELLA_HOME: '/x/y' }, '/home/u'), '/x/y');
  assert.equal(lib.configDir({}, '/home/u'), path.join('/home/u', '.umbrella'));
});

test('hasKey looks for key.json', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'umb-'));
  assert.equal(lib.hasKey({ UMBRELLA_HOME: dir }), false);
  fs.writeFileSync(path.join(dir, 'key.json'), '{}');
  assert.equal(lib.hasKey({ UMBRELLA_HOME: dir }), true);
});

test('resolveBinary prefers env, then resources, then dev locations', () => {
  const have = new Set(['/res/bin/umbrella', '/app/bin/umbrella']);
  const exists = (p) => have.has(p);
  assert.equal(lib.resolveBinary({ resourcesPath: '/res', appDir: '/app', env: {}, exists }), '/res/bin/umbrella');
  assert.equal(lib.resolveBinary({ appDir: '/app', env: {}, exists }), '/app/bin/umbrella');
  have.add('/custom');
  assert.equal(lib.resolveBinary({ resourcesPath: '/res', appDir: '/app', env: { UMBRELLA_BIN: '/custom' }, exists }), '/custom');
  assert.equal(lib.resolveBinary({ appDir: '/nowhere', env: {}, exists: () => false }), null);
});

test('parseHostEvent decodes what the Go client emits', () => {
  const payload = Buffer.from(JSON.stringify({ k: 'notify', kind: 'dm', from: 'Phát 🌂', text: 'chào' })).toString('base64');
  assert.deepEqual(lib.parseHostEvent(`umbrella;${payload}`), { k: 'notify', kind: 'dm', from: 'Phát 🌂', text: 'chào' });
  assert.equal(lib.parseHostEvent('other;abc'), null);
  assert.equal(lib.parseHostEvent('umbrella;@@@'), null);
  assert.equal(lib.parseHostEvent(undefined), null);
});

test('parseClipboardWrite decodes OSC 52 and ignores queries', () => {
  const b64 = Buffer.from('xin chào').toString('base64');
  assert.equal(lib.parseClipboardWrite(`c;${b64}`), 'xin chào');
  assert.equal(lib.parseClipboardWrite('c;?'), null);
  assert.equal(lib.parseClipboardWrite('nonsense'), null);
});

test('isNewer compares release tags', () => {
  assert.equal(lib.isNewer('v1.7.0', '1.6.1'), true);
  assert.equal(lib.isNewer('v1.6.1', '1.6.1'), false);
  assert.equal(lib.isNewer('v1.6.0', '1.6.1'), false);
  assert.equal(lib.isNewer('v2.0.0', '1.99.99'), true);
  assert.equal(lib.isNewer('latest', '1.6.1'), false);
  assert.equal(lib.isNewer('v1.7.0', '0.0.0-dev'), true);
});

test('notificationFor builds titles per kind', () => {
  assert.deepEqual(lib.notificationFor({ k: 'notify', kind: 'dm', from: 'Bob', text: 'hi' }), { title: 'Bob', body: 'hi' });
  assert.equal(lib.notificationFor({ k: 'notify', kind: 'mention', from: 'Bob', text: 'yo' }).title, 'Bob mentioned you');
  assert.match(lib.notificationFor({ k: 'notify', kind: 'buzz', from: 'Bob' }).title, /Bob buzzed you/);
  assert.equal(lib.notificationFor({ k: 'notify', kind: 'other' }), null);
  assert.equal(lib.notificationFor({ k: 'unread', n: 3 }), null);
});

test('windowTitle shows the unread count', () => {
  assert.equal(lib.windowTitle(0), 'Umbrella');
  assert.equal(lib.windowTitle(4), '(4) Umbrella');
});
