import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { copyFileSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { digest, inventory, verifySource } from './verify-source.mjs';

const source = fileURLToPath(new URL('../', import.meta.url));
const [target, ...args] = process.argv.slice(2);
assert(target, 'usage: pin-source.mjs APP_ROOT [--release TAG] [--binary PLATFORM=PATH]...');
const root = resolve(target);
const binaries = {};
let release;
for (let i = 0; i < args.length; i += 2) {
  const value = args[i + 1];
  assert(value, `missing value for ${args[i]}`);
  if (args[i] === '--release') {
    assert(!release, 'duplicate --release');
    release = value;
  } else {
    assert.equal(args[i], '--binary', `unknown option: ${args[i]}`);
    const split = value.indexOf('=');
    assert(split > 0, '--binary must be PLATFORM=PATH');
    const platform = value.slice(0, split);
    assert(['linux-arm64', 'linux-x64'].includes(platform), `unsupported binary platform: ${platform}`);
    assert(!binaries[platform], `duplicate binary platform: ${platform}`);
    const file = resolve(value.slice(split + 1));
    const bytes = readFileSync(file);
    assert(bytes.length >= 20, 'binary ELF header is incomplete');
    assert(bytes.subarray(0, 4).equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46])), 'binary must be ELF');
    assert.equal(bytes[4], 2, 'binary must be 64-bit ELF');
    assert.equal(bytes[5], 1, 'binary must be little-endian ELF');
    assert.equal(bytes.readUInt16LE(18), platform === 'linux-arm64' ? 183 : 62, 'binary platform differs from ELF machine');
    binaries[platform] = digest(file);
  }
}

const version = readFileSync(resolve(source, 'VERSION'), 'utf8').trim();
const git = (...parameters) => execFileSync('git', parameters, { cwd: source, encoding: 'utf8' }).trim();
const commit = git('rev-parse', 'HEAD');
if (release) {
  assert.equal(release, `v${version}`, 'release tag differs from VERSION');
  assert.equal(git('rev-parse', `${release}^{commit}`), commit, 'release tag differs from source HEAD');
  assert.equal(git('status', '--porcelain', '--untracked-files=normal'), '', 'release source must be clean');
}
verifySource(root, 'runtime');
const old = JSON.parse(readFileSync(resolve(root, 'effectgo.lock.json'), 'utf8'));
const hashes = (base, paths) => Object.fromEntries(paths.sort().map((path) => [path, digest(resolve(base, path))]));
const generatorFiles = hashes(source, inventory(source));
const runtimeFiles = hashes(resolve(source, 'rt'), inventory(resolve(source, 'rt')).filter((path) => !path.endsWith('_test.go')));
const tree = createHash('sha256').update(JSON.stringify(generatorFiles)).digest('hex');
const lock = {
  effect: '4.0.4',
  effectgo_version: version,
  upstream_commit: commit,
  source_tree_sha256: tree,
  snapshot_kind: release ? 'release' : 'release-candidate',
  ...(release ? { release_tag: release } : {}),
  source_tree_digest_format: 'sha256(JSON.stringify(generator_files with sorted paths))',
  generator_files: generatorFiles,
  runtime_files: runtimeFiles,
  tooling_files: { 'verify-source.mjs': digest(resolve(source, 'scripts/verify-source.mjs')) },
  binaries,
};
const runtimeRoot = resolve(root, 'backend/third_party/effectgo-rt');
for (const path of Object.keys(old.runtime_files)) {
  if (!Object.hasOwn(runtimeFiles, path)) rmSync(resolve(runtimeRoot, path));
}
for (const path of Object.keys(runtimeFiles)) {
  mkdirSync(dirname(resolve(runtimeRoot, path)), { recursive: true });
  copyFileSync(resolve(source, 'rt', path), resolve(runtimeRoot, path));
}
writeFileSync(resolve(runtimeRoot, 'SOURCE.json'), JSON.stringify({
  upstream_commit: commit,
  source_tree_sha256: tree,
  snapshot_kind: lock.snapshot_kind,
  effectgo_version: version,
  ...(release ? { release_tag: release } : {}),
  files: runtimeFiles,
}, null, 2) + '\n');
const tooling = resolve(root, 'scripts/vendor/effectgo');
mkdirSync(tooling, { recursive: true });
copyFileSync(resolve(source, 'scripts/verify-source.mjs'), resolve(tooling, 'verify-source.mjs'));
writeFileSync(resolve(root, 'scripts/effectgo-source.mjs'), `import { fileURLToPath } from 'node:url';
import { verifySource } from './vendor/effectgo/verify-source.mjs';
const root = fileURLToPath(new URL('../', import.meta.url));
verifySource(root, 'tooling');
verifySource(root, ...process.argv.slice(2));
`);
writeFileSync(resolve(root, 'effectgo.lock.json'), JSON.stringify(lock, null, 2) + '\n');
verifySource(root, 'tooling');
verifySource(root, 'runtime');
verifySource(root, 'source', source);
console.log(`Pinned effectgo ${version} (${lock.snapshot_kind}) tree=${tree} in ${root}`);
