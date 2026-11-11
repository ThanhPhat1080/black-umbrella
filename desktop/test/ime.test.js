'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { diff, afterKey } = require('../src/ime');

test('diff sends what turns the sent text into the textarea text', () => {
  assert.deepEqual(diff('', 'v'), { del: 0, add: 'v' });
  assert.deepEqual(diff('vie', 'viê'), { del: 1, add: 'ê' }); // Telex: e + e
  assert.deepEqual(diff('viê', 'việ'), { del: 1, add: 'ệ' }); // tone on the vowel
  assert.deepEqual(diff('việt nam đuo', 'việt nam đươ'), { del: 2, add: 'ươ' });
  assert.deepEqual(diff('ngươi', 'người'), { del: 2, add: 'ời' });
  assert.deepEqual(diff('việt', 'việt'), { del: 0, add: '' });
  assert.deepEqual(diff('abc', ''), { del: 3, add: '' });
  assert.deepEqual(diff('a🌂', 'a'), { del: 1, add: '' }); // astral characters are one Backspace
});

test('afterKey mirrors what xterm sent for a key', () => {
  assert.equal(afterKey('vi', 'e'), 'vie');
  assert.equal(afterKey('vie', '\x7f'), 'vi');
  assert.equal(afterKey('a🌂', '\x7f'), 'a');
  assert.equal(afterKey('xin', ' '), 'xin ');
  assert.equal(afterKey('xin', '\r'), null); // Enter: the client's line is gone
  assert.equal(afterKey('xin', '\x1b[D'), null); // arrow keys move the caret
  assert.equal(afterKey('xin', '\x1b[200~p\x1b[201~'), null); // a paste
  assert.equal(afterKey('xin', '\x03'), null); // Ctrl+C
});
