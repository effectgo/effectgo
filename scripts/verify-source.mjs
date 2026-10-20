import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

export const digest = (path) => createHash('sha256').update(readFileSync(path)).digest('hex');

export function inventory(directory, prefix = '') {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    if (['.git', 'node_modules', 'bin'].includes(entry.name)) return [];
    const name = prefix + entry.name;
    assert(!entry.isSymbolicLink(), `untracked source link: ${name}`);
    if (entry.isDirectory()) return inventory(resolve(directory, entry.name), name + '/');
    return /\.(go|stubs)$/.test(entry.name) || ['go.mod', 'go.sum'].includes(entry.name) ? [name] : [];
  });
}

export function verifySource(root, mode, directory) {
  const lock = JSON.parse(readFileSync(resolve(root, 'effectgo.lock.json'), 'utf8'));
  if (lock.generator_files) {
    const files = Object.fromEntries(Object.keys(lock.generator_files).sort().map((path) => [path, lock.generator_files[path]]));
    const tree = createHash('sha256').update(JSON.stringify(files)).digest('hex');
    assert.equal(tree, lock.source_tree_sha256, 'effectgo source tree digest differs from its file manifest');
  }
  if (lock.snapshot_kind === 'release') {
    assert.equal(lock.release_tag, `v${lock.effectgo_version}`, 'effectgo release tag differs from its version');
  }
  if (mode === 'binary') {
    const expected = lock.binaries[`${process.platform}-${process.arch}`];
    assert(expected, 'No pinned binary for this platform; build from EFFECTGO_DIR instead');
    assert.equal(digest(directory), expected, 'effectgo binary differs from the validated generator');
    return;
  }
  if (mode === 'tooling') {
    assert(lock.tooling_files, 'No pinned effectgo tooling; update the source lock');
    for (const [path, hash] of Object.entries(lock.tooling_files)) {
      assert.equal(digest(resolve(root, 'scripts/vendor/effectgo', path)), hash, `effectgo tooling drift: ${path}`);
    }
    return;
  }
  assert(mode === 'runtime' || mode === 'source', 'usage: verify-source.mjs APP_ROOT runtime|source|binary|tooling [path]');
  const base = mode === 'runtime' ? resolve(root, 'backend/third_party/effectgo-rt') : resolve(directory);
  const files = mode === 'runtime' ? lock.runtime_files : lock.generator_files;
  assert.deepEqual(inventory(base).sort(), Object.keys(files).sort(), `effectgo ${mode} build input inventory drift`);
  for (const [path, hash] of Object.entries(files)) {
    assert.equal(digest(resolve(base, path)), hash, `effectgo ${mode} source drift: ${path}`);
  }
  if (mode === 'runtime') {
    const source = JSON.parse(readFileSync(resolve(base, 'SOURCE.json'), 'utf8'));
    assert.equal(source.upstream_commit, lock.upstream_commit, 'effectgo runtime source commit drift');
    assert.equal(source.source_tree_sha256, lock.source_tree_sha256, 'effectgo runtime source tree drift');
    assert.equal(source.snapshot_kind, lock.snapshot_kind, 'effectgo runtime snapshot kind drift');
    assert.equal(source.effectgo_version, lock.effectgo_version, 'effectgo runtime version drift');
    assert.equal(source.release_tag, lock.release_tag, 'effectgo runtime release tag drift');
    assert.deepEqual(source.files, lock.runtime_files, 'effectgo runtime source manifest drift');
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [root, mode, directory] = process.argv.slice(2);
  assert(root, 'usage: verify-source.mjs APP_ROOT runtime|source|binary|tooling [path]');
  verifySource(resolve(root), mode, directory);
}
