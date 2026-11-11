'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { Session } = require('../src/session');

// A stand-in for the Go client: prints its arguments, creates key.json when registering, and
// exits as the FAKE variable says.
function setup(fake) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'umb-sess-'));
  const bin = path.join(dir, 'umbrella');
  fs.writeFileSync(
    bin,
    `#!/bin/sh
echo "ARGS[$*]"
[ "$1" = register ] && [ "$FAKE" != "cancel" ] && echo '{}' > "$UMBRELLA_HOME/key.json"
case "$FAKE" in
  fail) exit 3;;
  cancel) kill -INT $$; sleep 1;;
esac
exit 0
`,
    { mode: 0o755 },
  );
  const out = { data: '', quits: 0 };
  const session = new Session({
    bin,
    cwd: dir,
    env: { ...process.env, UMBRELLA_HOME: dir, FAKE: fake },
    onData: (d) => (out.data += d),
    onQuit: () => out.quits++,
  });
  return { session, out, dir };
}

const until = async (cond, ms = 5000) => {
  const t0 = Date.now();
  while (!cond()) {
    if (Date.now() - t0 > ms) throw new Error('timeout waiting for condition');
    await new Promise((r) => setTimeout(r, 20));
  }
};

test('registers first when there is no key, then starts the chat and quits on /quit', async () => {
  const { session, out } = setup('ok');
  session.start(80, 24);
  await until(() => out.quits === 1);
  assert.match(out.data, /ARGS\[register\]/);
  assert.match(out.data, /ARGS\[\]/);
  assert.ok(out.data.indexOf('ARGS[register]') < out.data.indexOf('ARGS[]'));
});

test('a failed run waits for Enter and then starts over', async () => {
  const { session, out, dir } = setup('fail');
  fs.writeFileSync(path.join(dir, 'key.json'), '{}'); // already registered: plain `umbrella`
  session.start(80, 24);
  await until(() => out.data.includes('Press Enter to try again'));
  assert.equal(out.quits, 0);
  session.write('x'); // anything but Enter is ignored
  const before = out.data.length;
  session.write('\r');
  await until(() => out.data.length > before && out.data.slice(before).includes('ARGS[]'));
});

test('a registration cancelled with Ctrl+C is a failure, not a success', async () => {
  const { session, out } = setup('cancel');
  session.start(80, 24);
  await until(() => out.data.includes('Press Enter to try again'));
  assert.equal(out.quits, 0);
  assert.equal(out.data.match(/ARGS\[register\]/g).length, 1, 'must not relaunch by itself');
});
