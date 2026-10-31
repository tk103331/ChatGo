#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p web
GOOS=js GOARCH=wasm go build -trimpath -o web/app.wasm ./cmd/web
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js
echo "built web/app.wasm"
