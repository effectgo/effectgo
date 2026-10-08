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

section "check: the normal run prints no signatures"
n=$(cd "$here/todo" && EFFECTGO_STUBS="$stdlib,$fuego" "$bin" ./... 2>&1 | grep -c "Effect<")
echo "lines with Effect<: $n"
