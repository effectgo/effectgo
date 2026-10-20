import assert from "node:assert/strict"
import { mkdtempSync, mkdirSync, writeFileSync, copyFileSync, symlinkSync, readFileSync, rmSync, unlinkSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import { spawnSync } from "node:child_process"
import test from "node:test"

function candidate(t) {
  const root = mkdtempSync(path.join(tmpdir(), "effectgo-release-check-"))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  for (const dir of ["scripts", "tools", "rt", ...["layers", "todo", "editor", "web/ts"].map(name => "examples/" + name)]) {
    mkdirSync(path.join(root, dir), { recursive: true })
  }
  copyFileSync(new URL("./release-check.sh", import.meta.url), path.join(root, "scripts/release-check.sh"))
  const deps = { effect: "4.0.4", "@effect/platform-node": "4.0.4" }
  const dev = { typescript: "7.0.2", tsx: "4.23.15" }
  const manifest = { dependencies: deps, devDependencies: dev }
  const packages = { "": manifest }
  for (const [name, version] of Object.entries({ ...deps, ...dev })) {
    const dir = path.join(root, "examples/web/ts/node_modules", name)
    mkdirSync(dir, { recursive: true })
    writeFileSync(path.join(dir, "package.json"), JSON.stringify({ version }))
    packages["node_modules/" + name] = { version }
  }
  const ts = path.join(root, "examples/web/ts")
  writeFileSync(path.join(ts, "package.json"), JSON.stringify(manifest))
  writeFileSync(path.join(ts, "package-lock.json"), JSON.stringify({ packages }))
  mkdirSync(path.join(ts, "node_modules/typescript/bin"))
  writeFileSync(path.join(ts, "node_modules/typescript/bin/tsc"), "")
  symlinkSync("/usr/bin/dirname", path.join(root, "tools/dirname"))
  const commands = {
    go: `if [ "$*" = 'env GOVERSION' ]; then printf '%s\\n' "\${TEST_GO_VERSION:-go1.26.9}"; exit; fi
[ "\${EFFECTGO_STRICT_CROSS_LANGUAGE:-}" = 1 ] || exit 98
printf 'go %s %s\\n' "\${PWD#"$TEST_ROOT"}" "$*" >> "$TEST_LOG"
[ "$*" != "\${TEST_FAIL_GO:-}" ]`,
    npm: `printf 'npm %s\\n' "$*" >> "$TEST_LOG"
[ "$*" != "\${TEST_FAIL_NPM:-}" ]`,
    node: `if [ "$1" = '--input-type=commonjs' ]; then exec "$TEST_NODE" "$@"; fi
printf 'node %s\n' "$*" >> "$TEST_LOG"
[ "$*" != "\${TEST_FAIL_NODE:-}" ]`,
    bash: `printf 'bash %s\\n' "$*" >> "$TEST_LOG"`,
  }
  for (const [name, body] of Object.entries(commands)) {
    writeFileSync(path.join(root, "tools", name), "#!/bin/sh\n" + body + "\n", { mode: 0o755 })
  }
  return {
    root,
    ts,
    run(overrides = {}) {
      const result = spawnSync("/bin/bash", [path.join(root, "scripts/release-check.sh")], {
        cwd: tmpdir(),
        encoding: "utf8",
        env: { ...process.env, PATH: path.join(root, "tools"), TEST_ROOT: root, TEST_LOG: path.join(root, "commands"), TEST_NODE: process.execPath, ...overrides },
      })
      return { ...result, output: result.stdout + result.stderr }
    },
    commands() { return readFileSync(path.join(root, "commands"), "utf8") },
  }
}


for (const missing of ["node", "npm"]) {
  test(`release check fails before Go checks when ${missing} is missing`, t => {
    const fixture = candidate(t)
    unlinkSync(path.join(fixture.root, "tools", missing))
    const result = fixture.run()
    assert.notEqual(result.status, 0)
    assert.match(result.output, new RegExp(missing + " is required"))
    assert.doesNotMatch(result.output, /all checks passed/)
  })
}

for (const dependency of ["effect", "@effect/platform-node", "typescript", "tsx"]) {
  test(`release check rejects missing installed ${dependency}`, t => {
    const fixture = candidate(t)
    unlinkSync(path.join(fixture.ts, "node_modules", dependency, "package.json"))
    const result = fixture.run()
    assert.notEqual(result.status, 0)
    assert.match(result.output, /npm ci/)
    assert.doesNotMatch(fixture.commands(), /go /)
  })
}

test("release check rejects lock drift and a missing compiler", async t => {
  for (const broken of ["lock", "compiler", "installed version", "Go version"]) {
    await t.test(broken, t => {
      const fixture = candidate(t)
      const env = {}
      if (broken === "lock") writeFileSync(path.join(fixture.ts, "package-lock.json"), "{}")
      if (broken === "compiler") unlinkSync(path.join(fixture.ts, "node_modules/typescript/bin/tsc"))
      if (broken === "installed version") writeFileSync(path.join(fixture.ts, "node_modules/effect/package.json"), '{"version":"4.0.5"}')
      if (broken === "Go version") env.TEST_GO_VERSION = "go1.27.0"
      const result = fixture.run(env)
      assert.notEqual(result.status, 0)
      assert.doesNotMatch(result.output, /all checks passed/)
    })
  }
})

test("release check propagates a decoder or tsc failure", async t => {
  for (const env of [{ TEST_FAIL_NODE: "scripts/test-isolated.mjs --root" }, { TEST_FAIL_NPM: "--prefix examples/web/ts run typecheck" }]) {
    await t.test(JSON.stringify(env), t => {
      const fixture = candidate(t)
      const result = fixture.run(env)
      assert.notEqual(result.status, 0)
      assert.doesNotMatch(result.output, /all checks passed/)
    })
  }
})
