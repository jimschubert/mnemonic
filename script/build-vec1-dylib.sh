#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
	echo "this script is for macOS users… check the releases page for pre-compiled linux extensions" >&2
	exit 1
fi

if ! command -v clang &>/dev/null; then
	echo "clang not found. did you install Xcode command line tools: xcode-select --install" >&2
	exit 1
fi

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/dist/vec1"
mkdir -p "$OUT_DIR"

MOD_DIR="$(cd "$ROOT_DIR" && go list -m -f '{{.Dir}}' github.com/ncruces/go-sqlite3-wasm/v2)"
BUILD_SH="${MOD_DIR}/vec1/build/build.sh"

if [[ ! -f "$BUILD_SH" ]]; then
	echo "could not find vec1 build script: $BUILD_SH" >&2
	exit 1
fi

RAW_URL="$(head -n 120 "$BUILD_SH" | grep 'sqlite.org/vec1/raw/' | sed -E 's/.*"(https:\/\/sqlite\.org\/vec1\/raw\/[^"]+)".*/\1/')"

if [[ -z "$RAW_URL" ]]; then
	echo "could not extract vec1 source url from $BUILD_SH" >&2
	exit 1
fi

VEC1_C="${OUT_DIR}/vec1.c"
VEC1_DYLIB="${OUT_DIR}/vec1.dylib"
VEC1_LOAD_PATH="${OUT_DIR}/vec1"
ARCH="$(uname -m)"

printf 'using vec1 source:\n  %s\n' "$RAW_URL"

curl -L --fail --silent --show-error "$RAW_URL" -o "$VEC1_C"

CFLAGS=(
	-g
	-O3
	-DNDEBUG
	-dynamiclib
	-undefined
	dynamic_lookup
	-fPIC
)

if [[ "$ARCH" == "x86_64" ]]; then
	CFLAGS+=(
		-mavx2
		-mfma
	)
fi

clang "${CFLAGS[@]}" -o "$VEC1_DYLIB" "$VEC1_C"

printf '\nbuilt:\n  %s\n' "$VEC1_DYLIB"
printf '\nload in sqlite3 with:\n  .load %s\n' "$VEC1_LOAD_PATH"

