import assert from 'node:assert/strict';
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { test } from 'node:test';
import { digest, verifySource } from './verify-source.mjs';

const source = fileURLToPath(new URL('../', import.meta.url));

test('pin and verify reject modified, additional, linked and inconsistent build inputs', (t) => {
  const root = mkdtempSync(resolve(tmpdir(), 'effectgo-source-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const runtime = resolve(root, 'backend/third_party/effectgo-rt');
  mkdirSync(runtime, { recursive: true });
  const baseline = { upstream_commit: 'baseline', source_tree_sha256: 'baseline', snapshot_kind: 'fixture', files: {} };
  writeFileSync(resolve(runtime, 'SOURCE.json'), JSON.stringify(baseline));
  writeFileSync(resolve(root, 'effectgo.lock.json'), JSON.stringify({ ...baseline, runtime_files: {}, binaries: {} }));
  execFileSync(process.execPath, [resolve(source, 'scripts/pin-source.mjs'), root]);
  verifySource(root, 'runtime');
  verifySource(root, 'source', source);
  verifySource(root, 'tooling');
  const lock = JSON.parse(readFileSync(resolve(root, 'effectgo.lock.json'), 'utf8'));
  assert.equal(lock.snapshot_kind, 'release-candidate');
  const goMod = resolve(runtime, 'go.mod');
  const original = readFileSync(goMod);
  writeFileSync(goMod, 'module modified\n');
  assert.throws(() => verifySource(root, 'runtime'));
  assert.throws(() => execFileSync(process.execPath, [resolve(source, 'scripts/pin-source.mjs'), root], { stdio: 'pipe' }));
  writeFileSync(goMod, original);
  writeFileSync(resolve(runtime, 'extra.go'), 'package extra');
  assert.throws(() => verifySource(root, 'runtime'));
  rmSync(resolve(runtime, 'extra.go'));
  symlinkSync(goMod, resolve(runtime, 'linked.go'));
  assert.throws(() => verifySource(root, 'runtime'));
  rmSync(resolve(runtime, 'linked.go'));
  const descriptor = readFileSync(resolve(runtime, 'SOURCE.json'));
  writeFileSync(resolve(runtime, 'SOURCE.json'), JSON.stringify({ ...JSON.parse(descriptor), upstream_commit: 'wrong' }));
  assert.throws(() => verifySource(root, 'runtime'));
  writeFileSync(resolve(runtime, 'SOURCE.json'), descriptor);
  const tooling = resolve(root, 'scripts/vendor/effectgo/verify-source.mjs');
  const toolBytes = readFileSync(tooling);
  writeFileSync(tooling, '// modified tool');
  assert.throws(() => verifySource(root, 'tooling'));
  writeFileSync(tooling, toolBytes);
  const binary = resolve(root, 'binary');
  writeFileSync(binary, 'validated binary');
  lock.binaries[`${process.platform}-${process.arch}`] = digest(binary);
  writeFileSync(resolve(root, 'effectgo.lock.json'), JSON.stringify(lock));
  verifySource(root, 'binary', binary);
  writeFileSync(binary, 'different binary');
  assert.throws(() => verifySource(root, 'binary', binary));
  const copiedSource = resolve(root, 'source');
  for (const path of Object.keys(lock.generator_files)) {
    mkdirSync(dirname(resolve(copiedSource, path)), { recursive: true });
    copyFileSync(resolve(source, path), resolve(copiedSource, path));
  }
  verifySource(root, 'source', copiedSource);
  writeFileSync(resolve(copiedSource, 'extra.go'), 'package extra');
  assert.throws(() => verifySource(root, 'source', copiedSource));
  verifySource(root, 'runtime');
  for (const [elfClass, machine] of [[1, 183], [2, 62]]) {
    const bytes = Buffer.alloc(20);
    Buffer.from([0x7f, 0x45, 0x4c, 0x46, elfClass, 1]).copy(bytes);
    bytes.writeUInt16LE(machine, 18);
    writeFileSync(binary, bytes);
    assert.throws(() => execFileSync(process.execPath, [resolve(source, 'scripts/pin-source.mjs'), root, '--binary', `linux-arm64=${binary}`], { stdio: 'pipe' }));
  }
});
