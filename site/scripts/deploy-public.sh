#!/usr/bin/env bash
# Build, check and deploy the PUBLIC docs site (journal excluded, private
# project names redacted) to Cloudflare (Pages on Workers) with the `cf` CLI.
#
# The deploy project in site/deploy/ contains only config; its assets
# directory (site/deploy/public/) is filled from a checked public build, so
# `cf deploy` can never build or upload the unredacted default site.
set -euo pipefail
cd "$(dirname "$0")/.."

SITE_URL="${SITE_URL:-https://effectgo-docs.faintghost.workers.dev}"

rm -rf dist
SITE_URL="$SITE_URL" pnpm build:public
pnpm check:public            # exits non-zero on any denylisted term

rm -rf deploy/public
cp -r dist deploy/public

cd deploy
pnpm install
cf deploy "$@"
