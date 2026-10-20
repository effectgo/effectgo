#!/usr/bin/env bash
# Validate a release candidate without installing tools or publishing artifacts.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

fail() { printf 'release-check: %s\n' "$*" >&2; exit 1; }
for tool in go node npm; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is required; install the documented toolchain first"
done
case "$(go env GOVERSION)" in
  go1.26.*) ;;
  *) fail "Go 1.26.x is required" ;;
esac
npm --version >/dev/null

node --input-type=commonjs <<'NODE'
const fs = require("node:fs")
const path = require("node:path")
const root = path.resolve("examples/web/ts")
function fail(message) {
  console.error("release-check: " + message + "; run npm ci --prefix examples/web/ts with the committed lockfile")
  process.exit(1)
}
function json(file) {
  try { return JSON.parse(fs.readFileSync(file, "utf8")) }
  catch { fail("missing or invalid " + file) }
}
const manifest = json(path.join(root, "package.json"))
const lock = json(path.join(root, "package-lock.json"))
for (const [name, version] of Object.entries({effect: "4.0.4", "@effect/platform-node": "4.0.4", typescript: "7.0.2", tsx: "4.23.15"})) {
  const group = name === "typescript" || name === "tsx" ? "devDependencies" : "dependencies"
  if (manifest[group]?.[name] !== version || lock.packages?.[""]?.[group]?.[name] !== version || lock.packages?.["node_modules/" + name]?.version !== version) {
    fail(name + " must be pinned to " + version + " in both package.json and package-lock.json")
  }
  if (json(path.join(root, "node_modules", name, "package.json")).version !== version) {
    fail("installed " + name + " does not match " + version)
  }
}
if (!fs.existsSync(path.join(root, "node_modules/typescript/bin/tsc"))) fail("the locked TypeScript compiler is missing")
if (typeof fetch !== "function" || typeof File !== "function") fail("Node.js must provide fetch and File (use Node.js 22 or newer)")
NODE

export EFFECTGO_STRICT_CROSS_LANGUAGE=1
export GOWORK=off
# The candidate must be checked afresh, including every real decoder and tsc test.
node --test scripts/release-check.test.mjs scripts/source-tools.test.mjs scripts/test-isolated.test.mjs
go build ./...
go vet ./...
node scripts/test-isolated.mjs --root
(
  cd rt
  go vet ./...
  go test -race ./... -count=1
)
for example in layers todo editor web; do
  (
    cd "examples/$example"
    go vet ./...
    go test ./... -count=1
  )
done
bash examples/run.sh
npm --prefix examples/web/ts run typecheck
printf 'release-check: all checks passed\n'
