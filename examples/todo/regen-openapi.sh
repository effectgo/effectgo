#!/bin/sh
# Regenerates testdata/openapi, the inputs of the OpenAPI tests:
#   fuego.json           the document that Fuego generates for the server
#   effectgo-merged.json fuego.json with the error responses that effectgo infers
# Run it after changing the handlers, the errmap or the server options.
set -e
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
cd "$here"
(cd "$root" && GOFLAGS=-buildvcs=false go build -o bin/effectgo ./cmd/effectgo)
stubs="$root/stubs/stdlib.stubs,$root/stubs/fuego.stubs"
mkdir -p testdata/openapi
GOFLAGS=-buildvcs=false go run ./cmd/openapispec -o testdata/openapi/fuego.json
"$root/bin/effectgo" openapi -stubs="$stubs" -merge testdata/openapi/fuego.json ./... > testdata/openapi/effectgo-merged.json
echo "regenerated testdata/openapi/fuego.json and testdata/openapi/effectgo-merged.json"
