#!/usr/bin/env bash
# Package ChatGo as a native desktop app with the MyGo toolchain.
#
#   ./build.sh                    # build for the current OS/arch
#   ./build.sh -platform darwin/universal,linux/amd64,windows/amd64
#   ./build.sh -skip-dmg          # skip the macOS disk image
#
# All arguments are passed through to `mygo build`.
set -euo pipefail
cd "$(dirname "$0")"

rm -rf dist/.staging-*

# The mygo CLI is not declared as a tool dependency, so build it from the module.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mygo_bin="$work/mygo"
echo "[build] compiling mygo CLI…"
go build -o "$mygo_bin" github.com/egoist/mygo/cmd/mygo

echo "[build] packaging ChatGo…"
"$mygo_bin" build "$@"

echo "[build] done: see $(pwd)/dist"
