import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

test('isolated runner discovers every test and propagates a real test failure', t => {
  const cwd = mkdtempSync(join(tmpdir(), 'effectgo-isolated-check-'));
  t.after(() => rmSync(cwd, { recursive: true, force: true }));
  writeFileSync(join(cwd, 'go.mod'), 'module example.com/isolated\n\ngo 1.26\n');
  writeFileSync(join(cwd, 'suite_test.go'), `package isolated
import ("os"; "testing")
var state bool
func TestFirst(t *testing.T) { if state { t.Fatal("shared process") }; state = true }
func TestSecond(t *testing.T) {
 if state { t.Fatal("shared process") }; state = true
 if os.Getenv("FAIL_SECOND") == "1" { t.Fatal("consumer-visible failure") }
}
`);
  const runner = fileURLToPath(new URL('./test-isolated.mjs', import.meta.url));
  for (const fail of [false, true]) {
    const result = spawnSync(process.execPath, [runner, '.'], {
      cwd, encoding: 'utf8', timeout: 120000,
      env: { ...process.env, GOWORK: 'off', FAIL_SECOND: fail ? '1' : '0' },
    });
    const output = result.stdout + result.stderr;
    if (fail) {
      assert.notEqual(result.status, 0, output);
      assert.match(output, /consumer-visible failure/);
    } else {
      assert.equal(result.status, 0, output);
      assert.match(output, /2 top-level tests passed/);
    }
  }
});
