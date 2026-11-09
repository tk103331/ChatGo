#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p web
GOOS=js GOARCH=wasm go build -trimpath -o web/app.wasm ./cmd/web
# The toolchain's copy is read-only, and cp will not overwrite a read-only
# destination, so drop the previous one before copying.
rm -f web/wasm_exec.js
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js
echo "built web/app.wasm"
