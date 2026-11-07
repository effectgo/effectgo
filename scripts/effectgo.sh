#!/usr/bin/env bash
set -euo pipefail

effectgo_release_binary() {
  local root=$1 tag arch binary url
  tag="$(node -e 'const lock = require(process.argv[1]); if (!lock.release_tag) process.exit(1); process.stdout.write(lock.release_tag)' "$root/effectgo.lock.json")" || {
    echo "effectgo.lock.json pins no release; set EFFECTGO_BIN or EFFECTGO_DIR." >&2
    return 1
  }
  case "$(uname -m)" in
    aarch64 | arm64) arch=arm64 ;;
    x86_64 | amd64) arch=amd64 ;;
    *) echo "no effectgo release binary for $(uname -m); set EFFECTGO_DIR." >&2; return 1 ;;
  esac
  binary="${EFFECTGO_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/effectgo/releases}/$tag/effectgo-linux-$arch"
  if [[ ! -f "$binary" ]]; then
    url="https://github.com/effectgo/effectgo/releases/download/$tag/effectgo-linux-$arch"
    mkdir -p "$(dirname "$binary")"
    if ! curl -fsSL --retry 3 -o "$binary.part" "$url"; then
      rm -f "$binary.part"
      echo "could not download $url; set EFFECTGO_BIN or EFFECTGO_DIR." >&2
      return 1
    fi
    node "$root/scripts/effectgo-source.mjs" binary "$binary.part" || { rm -f "$binary.part"; return 1; }
    chmod +x "$binary.part"
    mv "$binary.part" "$binary"
  fi
  printf '%s\n' "$binary"
}

# Sourced by consumer scripts to share one call-owned build.
effectgo_prepare() {
  local root
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  node "$root/scripts/effectgo-source.mjs" tooling || return 2
  if [[ -z "${EFFECTGO_BIN:-}" && -z "${EFFECTGO_DIR:-}" ]]; then
    EFFECTGO_BIN="$(effectgo_release_binary "$root")" || return 2
  fi
  if [[ -n "${EFFECTGO_BIN:-}" ]]; then
    node "$root/scripts/effectgo-source.mjs" binary "$EFFECTGO_BIN" || return 2
    effectgo_command="$EFFECTGO_BIN"
  else
    node "$root/scripts/effectgo-source.mjs" source "$EFFECTGO_DIR" || return 2
    effectgo_command="$1/effectgo"
    env GOWORK=off GOFLAGS=-buildvcs=false go build -C "$EFFECTGO_DIR" -o "$effectgo_command" ./cmd/effectgo
  fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  tool_dir="$(mktemp -d)"
  trap 'rm -rf "$tool_dir"' EXIT
  effectgo_prepare "$tool_dir"
  "$effectgo_command" "$@"
fi
