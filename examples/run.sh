#!/usr/bin/env bash
# Builds bin/effectgo and runs it on the two examples, standalone and through
# go vet, with the shipped stub files. Read-only on the examples: it writes only
# bin/ and the Go build cache.
#
#   examples/layers   effects: contract violation, interface conformance, package-pure
#   examples/todo     errset: the ErrConflict regression and the opaque body findings
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"
export GOFLAGS=-buildvcs=false

bin="$root/bin/effectgo"
stdlib="$root/stubs/stdlib.stubs"
fuego="$root/stubs/fuego.stubs"

mkdir -p "$root/bin"
(cd "$root" && go build -o "$bin" ./cmd/effectgo) || { echo "build failed" >&2; exit 1; }

section() { printf '\n== %s\n' "$*"; }

section "layers, standalone: effectgo -stubs=stubs/stdlib.stubs ./..."
(cd "$here/layers" && "$bin" -stubs="$stdlib" ./...); echo "exit $?"

section "layers, go vet: go vet -vettool=bin/effectgo -stubs=stubs/stdlib.stubs ./..."
(cd "$here/layers" && go vet -vettool="$bin" -stubs="$stdlib" ./...); echo "exit $?"

section "layers, standalone with -effects.report (first 6 lines)"
(cd "$here/layers" && "$bin" -effects.report -stubs="$stdlib" ./... 2>&1 | head -6); echo

section "todo, standalone: EFFECTGO_STUBS=stdlib,fuego effectgo ./..."
(cd "$here/todo" && EFFECTGO_STUBS="$stdlib,$fuego" "$bin" ./...); echo "exit $?"

section "todo, go vet: go vet -vettool=bin/effectgo -stubs=stdlib,fuego ./..."
(cd "$here/todo" && go vet -vettool="$bin" -stubs="$stdlib,$fuego" ./...); echo "exit $?"

section "todo, go vet with -errset.ignoreopaque (drops the opaque body findings)"
(cd "$here/todo" && go vet -vettool="$bin" -stubs="$stdlib,$fuego" -errset.ignoreopaque ./...); echo "exit $?"

section "todo, signatures: effectgo sig ./... (EFFECTGO_STUBS)"
(cd "$here/todo" && EFFECTGO_STUBS="$stdlib,$fuego" "$bin" sig ./... 2>&1 | grep -E "(Service\.Update|Store\.modify|v3Update):"); echo

section "editor, zero configuration: effectgo ./... (no -stubs, no EFFECTGO_STUBS; the project file effectgo.stubs is read)"
(cd "$here/editor" && env -u EFFECTGO_STUBS "$bin" ./...); echo "exit $?"

section "editor, zero configuration through go vet"
(cd "$here/editor" && env -u EFFECTGO_STUBS go vet -vettool="$bin" ./...); echo "exit $?"

section "check: the normal run prints no signatures"
n=$(cd "$here/todo" && EFFECTGO_STUBS="$stdlib,$fuego" "$bin" ./... 2>&1 | grep -c "Effect<")
echo "lines with Effect<: $n"

fail=0

section "web, standalone: effectgo -test=false ./... (expected 0 findings; tests inject unmapped failures)"
(cd "$here/web" && "$bin" -test=false ./...); rc=$?; echo "exit $rc"
[ "$rc" -eq 0 ] || fail=1

section "web, generated TypeScript: effectgo ts -name contacts ./... against web/ts/api.gen.ts"
gen="$(mktemp)"
(cd "$here/web" && "$bin" ts -name contacts ./... > "$gen"); rc=$?; echo "exit $rc"
if [ "$rc" -ne 0 ]; then
  fail=1
elif diff -u "$here/web/ts/api.gen.ts" "$gen"; then
  echo "api.gen.ts matches the generator"
else
  echo "api.gen.ts differs from the generator; regenerate: cd examples/web && effectgo ts -name contacts -o ts/api.gen.ts ./..."
  fail=1
fi
rm -f "$gen"

exit "$fail"
