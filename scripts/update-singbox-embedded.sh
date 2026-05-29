#!/usr/bin/env bash
set -euo pipefail

usage() {
    echo "Usage: $0 <mipsel-3.4|mips-3.4|aarch64-3.10> [sing-box-version]" >&2
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

ENTWARE_ARCH="${1:-}"
SINGBOX_VERSION_ARG="${2:-}"
if [[ -z "$ENTWARE_ARCH" || $# -lt 1 || $# -gt 2 ]]; then
    usage
    exit 1
fi

EMBEDDED_GO="$PROJECT_ROOT/internal/singbox/installer/embedded.go"
DEFAULT_SINGBOX_VERSION="$(sed -n 's/^const RequiredVersion = "\(.*\)"/\1/p' "$EMBEDDED_GO")"
SINGBOX_VERSION="${SINGBOX_VERSION_ARG:-${SINGBOX_VERSION:-$DEFAULT_SINGBOX_VERSION}}"
RELEASE_REPO="${RELEASE_REPO:-miuirussia/awg-manager}"
RELEASE_TAG="${RELEASE_TAG:-latest}"
RELEASE_BASE_URL="${RELEASE_BASE_URL:-https://github.com/$RELEASE_REPO/releases/download/$RELEASE_TAG}"

case "$ENTWARE_ARCH" in
    mipsel-3.4|mips-3.4|aarch64-3.10) ;;
    *)
        echo "Unknown architecture: $ENTWARE_ARCH" >&2
        usage
        exit 1
        ;;
esac

if [[ -z "$SINGBOX_VERSION" ]]; then
    echo "ERROR: unable to determine sing-box version from $EMBEDDED_GO" >&2
    exit 1
fi

require_command() {
    local name="$1"
    if ! command -v "$name" >/dev/null 2>&1; then
        echo "ERROR: missing required command: $name" >&2
        exit 1
    fi
}

sha256_file() {
    local path="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$path" | awk '{print $1}'
    else
        shasum -a 256 "$path" | awk '{print $1}'
    fi
}

file_size() {
    local path="$1"
    if stat -c '%s' "$path" >/dev/null 2>&1; then
        stat -c '%s' "$path"
    else
        stat -f '%z' "$path"
    fi
}

require_command gofmt
require_command python3

cd "$PROJECT_ROOT"

OUTPUT="$PROJECT_ROOT/dist/sing-box-$SINGBOX_VERSION-$ENTWARE_ARCH"
if [[ ! -f "$OUTPUT" ]]; then
    echo "ERROR: missing sing-box binary: $OUTPUT" >&2
    exit 1
fi

OUTPUT_SHA256="$(sha256_file "$OUTPUT")"
OUTPUT_SIZE="$(file_size "$OUTPUT")"
# Sidecar for independent integrity checks when mirroring to a package repo.
printf '%s\n' "$OUTPUT_SHA256" > "${OUTPUT}.sha256"
OUTPUT_URL="$RELEASE_BASE_URL/$(basename "$OUTPUT")"

EMBEDDED_GO="$EMBEDDED_GO" \
SINGBOX_VERSION="$SINGBOX_VERSION" \
ENTWARE_ARCH="$ENTWARE_ARCH" \
OUTPUT_URL="$OUTPUT_URL" \
OUTPUT_SHA256="$OUTPUT_SHA256" \
OUTPUT_SIZE="$OUTPUT_SIZE" \
python3 <<'PY'
import os
import pathlib
import re
import sys

path = pathlib.Path(os.environ["EMBEDDED_GO"])
version = os.environ["SINGBOX_VERSION"]
arch = os.environ["ENTWARE_ARCH"]
url = os.environ["OUTPUT_URL"]
sha256 = os.environ["OUTPUT_SHA256"]
size = os.environ["OUTPUT_SIZE"]

text = path.read_text()
text = re.sub(
    r'const RequiredVersion = "([^"]*)"',
    f'const RequiredVersion = "{version}"',
    text,
    count=1,
)

entry_pattern = re.compile(
    rf'(\t"{re.escape(arch)}":\s*)'
    r'\{Version: RequiredVersion, URL: "[^"]*", SHA256: "[^"]*"(?:, Size: \d+)?\},'
)
replacement = (
    rf'\1{{Version: RequiredVersion, URL: "{url}", SHA256: "{sha256}", Size: {size}}},'
)
text, count = entry_pattern.subn(replacement, text, count=1)
if count != 1:
    sys.stderr.write(f"ERROR: unable to update EmbeddedBinaries entry for {arch}\n")
    sys.exit(1)

path.write_text(text)
PY

gofmt -w "$EMBEDDED_GO"
echo "Updated $EMBEDDED_GO for $ENTWARE_ARCH"
