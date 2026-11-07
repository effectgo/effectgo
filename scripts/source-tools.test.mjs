import assert from 'node:assert/strict';
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
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
  execFileSync(process.execPath, [resolve(source, 'scripts/pin-source.mjs'), root]);
  verifySource(root, 'source', source);
  verifySource(root, 'tooling');
  const lock = JSON.parse(readFileSync(resolve(root, 'effectgo.lock.json'), 'utf8'));
  assert.equal(lock.snapshot_kind, 'release-candidate');
  assert.equal(Object.hasOwn(lock, 'runtime_files'), false);
  assert.equal(existsSync(resolve(root, 'backend/third_party/effectgo-rt')), false);
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
  const original = readFileSync(resolve(copiedSource, 'go.mod'));
  writeFileSync(resolve(copiedSource, 'go.mod'), 'module modified\n');
  assert.throws(() => verifySource(root, 'source', copiedSource));
  writeFileSync(resolve(copiedSource, 'go.mod'), original);
  symlinkSync(resolve(copiedSource, 'go.mod'), resolve(copiedSource, 'linked.go'));
  assert.throws(() => verifySource(root, 'source', copiedSource));
  rmSync(resolve(copiedSource, 'linked.go'));
  writeFileSync(resolve(copiedSource, 'extra.go'), 'package extra');
  assert.throws(() => verifySource(root, 'source', copiedSource));
  for (const [elfClass, machine] of [[1, 183], [2, 62]]) {
    const bytes = Buffer.alloc(20);
    Buffer.from([0x7f, 0x45, 0x4c, 0x46, elfClass, 1]).copy(bytes);
    bytes.writeUInt16LE(machine, 18);
    writeFileSync(binary, bytes);
    assert.throws(() => execFileSync(process.execPath, [resolve(source, 'scripts/pin-source.mjs'), root, '--binary', `linux-arm64=${binary}`], { stdio: 'pipe' }));
  }
});

test('generated launcher downloads, caches and verifies the pinned release with overrides', (t) => {
  const root = mkdtempSync(resolve(tmpdir(), 'effectgo-launcher-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  execFileSync(process.execPath, [resolve(source, 'scripts/pin-source.mjs'), root]);
  const lockPath = resolve(root, 'effectgo.lock.json');
  const lock = JSON.parse(readFileSync(lockPath, 'utf8'));
  const binary = resolve(root, 'release-binary');
  writeFileSync(binary, '#!/usr/bin/env bash\nprintf "release:%s\\n" "$*"\n');
  chmodSync(binary, 0o755);
  lock.release_tag = `v${lock.effectgo_version}`;
  lock.binaries[`${process.platform}-${process.arch}`] = digest(binary);
  writeFileSync(lockPath, JSON.stringify(lock));
  const stubs = resolve(root, 'stubs');
  mkdirSync(stubs);
  const curl = resolve(stubs, 'curl');
  writeFileSync(curl, '#!/usr/bin/env bash\nset -eu\nprintf "%s\\n" "$*" >> "$DOWNLOAD_LOG"\nwhile [[ "$1" != -o ]]; do shift; done\ncp "$RELEASE_BINARY" "$2"\n');
  chmodSync(curl, 0o755);
  const log = resolve(root, 'downloads');
  const env = { ...process.env, PATH: `${stubs}:${process.env.PATH}`, EFFECTGO_CACHE_DIR: resolve(root, 'cache'), EFFECTGO_BIN: '', EFFECTGO_DIR: '', DOWNLOAD_LOG: log, RELEASE_BINARY: binary };
  const launch = (extra = {}) => execFileSync('bash', [resolve(root, 'scripts/effectgo.sh'), 'ts', '--help'], { env: { ...env, ...extra }, encoding: 'utf8', stdio: 'pipe' });
  assert.equal(launch(), 'release:ts --help\n');
  const download = readFileSync(log, 'utf8');
  const arch = process.arch === 'x64' ? 'amd64' : 'arm64';
  assert(download.includes(`https://github.com/effectgo/effectgo/releases/download/${lock.release_tag}/effectgo-linux-${arch}`));
  assert.equal(launch(), 'release:ts --help\n');
  assert.equal(readFileSync(log, 'utf8'), download, 'cache hit must not download');
  const cached = resolve(env.EFFECTGO_CACHE_DIR, lock.release_tag, `effectgo-linux-${arch}`);
  writeFileSync(cached, '#!/usr/bin/env bash\necho tampered\n');
  assert.throws(() => launch(), /binary differs/);
  assert.equal(launch({ EFFECTGO_BIN: binary }), 'release:ts --help\n');
  assert.equal(readFileSync(log, 'utf8'), download);
  rmSync(cached);
  writeFileSync(binary, '#!/usr/bin/env bash\necho corrupt download\n');
  assert.throws(() => launch(), /binary differs/);
  assert.equal(existsSync(cached), false);
  assert.equal(existsSync(`${cached}.part`), false);
});
