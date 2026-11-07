#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
node "$root/scripts/effectgo-source.mjs" runtime
bash "$root/scripts/effectgo.sh" ts -name fixture -o "$root/frontend/client.gen.ts" ./...
