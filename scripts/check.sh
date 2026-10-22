#!/usr/bin/env bash
# Routine feedback; the complete race and cross-language gate is release-check.sh.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
source "$root/scripts/check-env.sh"
for tool in go node python3 pnpm; do
  command -v "$tool" >/dev/null 2>&1 || { printf 'check: %s is required\n' "$tool" >&2; exit 1; }
done
node --test scripts/source-tools.test.mjs scripts/release-check.test.mjs scripts/test-isolated.test.mjs
python3 scripts/benchmark_test.py
go build ./...
go vet ./...
pnpm --dir site run lint:docs
SITE_URL="${SITE_URL:-https://effectgo-docs.faintghost.workers.dev}" pnpm --dir site run build:public
pnpm --dir site run check:public
printf 'check: all routine checks passed (full release gate: bash scripts/release-check.sh)\n'
