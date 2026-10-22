import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// Analyzer fixtures retain race-detector metadata across package loads. A fresh
// process per discovered test bounds that memory without selecting a subset.
const packages = process.argv.slice(2);
assert(packages.length > 0, 'usage: node scripts/test-isolated.mjs PACKAGE...');
process.env.GOMAXPROCS ||= '2';
process.env.GOMEMLIMIT ||= '2GiB';
process.env.GOFLAGS = `${process.env.GOFLAGS ? process.env.GOFLAGS + ' ' : ''}-p=1`;
const directory = mkdtempSync(join(tmpdir(), 'effectgo-race-'));
function run(command, args, options = {}) {
  const result = spawnSync(command, args, { stdio: 'inherit', ...options });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} ${args.join(' ')} failed (${result.signal ?? result.status})`);
  return result.stdout;
}
try {
  if (packages.length === 1 && packages[0] === '--root') {
    const all = run('go', ['list', './...'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] }).trim().split('\n');
    const isolated = all.filter(pkg => ['/cmd/effectgo', '/tsgen', '/errset', '/openapi', '/signature'].some(suffix => pkg.endsWith(suffix)));
    const ordinary = all.filter(pkg => !isolated.includes(pkg));
    assert.equal(isolated.length, 5, 'expected CLI, generator, error-set, OpenAPI and signature test packages');
    if (ordinary.length > 0) run('go', ['test', '-race', '-count=1', ...ordinary]);
    packages.splice(0, 1, ...isolated);
  }
  for (const pkg of packages) {
    const cwd = run('go', ['list', '-f', '{{.Dir}}', pkg], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] }).trim();
    const binary = join(directory, 'suite');
    run('go', ['test', '-race', '-c', '-o', binary, pkg]);
    const list = run(binary, ['-test.list=.'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] });
    const tests = list.split('\n').filter(name => /^(Test|Example|Fuzz)/.test(name));
    assert(tests.length > 0, `${pkg}: no tests discovered`);
    for (const name of tests) {
      console.log(`isolated race: ${pkg} ${name}`);
      run(binary, [`-test.run=^${name}$`, '-test.timeout=10m'], { cwd });
    }
    console.log(`isolated race: ${pkg}: ${tests.length} top-level tests passed`);
    rmSync(binary);
  }
} finally {
  rmSync(directory, { recursive: true, force: true });
}
