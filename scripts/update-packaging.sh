#!/usr/bin/env bash
# Update packaging/scoop and packaging/aur/creel-bin for a GitHub Release tag.
#
# Usage:
#   ./scripts/update-packaging.sh           # latest release
#   ./scripts/update-packaging.sh v0.5.0
#
# After running:
#   - commit the packaging/ changes in this repo
#   - sync Scoop: copy packaging/scoop/creel.json into rsiota/scoop-creel
#     (or run with --push-scoop when SCOOP_BUCKET_DIR is a local clone)
#   - sync AUR: copy packaging/aur/creel-bin/* into the creel-bin AUR checkout
#     and push (requires an AUR account; see packaging/README.md)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REPO="${CREEL_REPO:-rsiota/creel}"
TAG="${1:-}"
PUSH_SCOOP=0
for arg in "$@"; do
  case "$arg" in
    --push-scoop) PUSH_SCOOP=1 ;;
    v*) TAG="$arg" ;;
  esac
done

if [[ -z "$TAG" ]]; then
  TAG="$(gh release view --repo "$REPO" --json tagName -q .tagName)"
fi
if [[ "$TAG" != v* ]]; then
  echo "tag must look like v0.5.0 (got: $TAG)" >&2
  exit 1
fi
VERSION="${TAG#v}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Fetching checksums for ${TAG}..."
gh release download "$TAG" --repo "$REPO" -p checksums.txt -O "$TMP/checksums.txt"

hash_for() {
  local file="$1"
  local line
  line="$(grep -E "  ${file}$" "$TMP/checksums.txt" || true)"
  if [[ -z "$line" ]]; then
    echo "missing checksum for $file in $TAG checksums.txt" >&2
    exit 1
  fi
  awk '{print $1}' <<<"$line"
}

WIN_AMD64="creel_${VERSION}_Windows_x86_64.zip"
WIN_ARM64="creel_${VERSION}_Windows_arm64.zip"
LIN_AMD64="creel_${VERSION}_Linux_x86_64.tar.gz"
LIN_ARM64="creel_${VERSION}_Linux_arm64.tar.gz"

WIN_AMD64_HASH="$(hash_for "$WIN_AMD64")"
WIN_ARM64_HASH="$(hash_for "$WIN_ARM64")"
LIN_AMD64_HASH="$(hash_for "$LIN_AMD64")"
LIN_ARM64_HASH="$(hash_for "$LIN_ARM64")"

BASE="https://github.com/${REPO}/releases/download/${TAG}"

cat >"$ROOT/packaging/scoop/creel.json" <<EOF
{
    "version": "${VERSION}",
    "description": "Fast, vim-driven SQL TUI for SQLite, MySQL, and PostgreSQL",
    "homepage": "https://github.com/${REPO}",
    "license": "MIT",
    "architecture": {
        "64bit": {
            "url": "${BASE}/${WIN_AMD64}",
            "hash": "${WIN_AMD64_HASH}"
        },
        "arm64": {
            "url": "${BASE}/${WIN_ARM64}",
            "hash": "${WIN_ARM64_HASH}"
        }
    },
    "bin": "creel.exe",
    "checkver": {
        "github": "https://github.com/${REPO}"
    },
    "autoupdate": {
        "architecture": {
            "64bit": {
                "url": "https://github.com/${REPO}/releases/download/v\$version/creel_\$version_Windows_x86_64.zip"
            },
            "arm64": {
                "url": "https://github.com/${REPO}/releases/download/v\$version/creel_\$version_Windows_arm64.zip"
            }
        }
    }
}
EOF

cat >"$ROOT/packaging/aur/creel-bin/PKGBUILD" <<EOF
# Maintainer: Ruben Siota Perez <rsiota@gmail.com>
# Prebuilt binary package. Source builds can use \`go install\` or a future \`creel\` PKGBUILD.
pkgname=creel-bin
pkgver=${VERSION}
pkgrel=1
pkgdesc='Fast, vim-driven SQL TUI for SQLite, MySQL, and PostgreSQL'
arch=('x86_64' 'aarch64')
url='https://github.com/${REPO}'
license=('MIT')
provides=('creel')
conflicts=('creel')
source_x86_64=("https://github.com/${REPO}/releases/download/v\${pkgver}/creel_\${pkgver}_Linux_x86_64.tar.gz")
source_aarch64=("https://github.com/${REPO}/releases/download/v\${pkgver}/creel_\${pkgver}_Linux_arm64.tar.gz")
sha256sums_x86_64=('${LIN_AMD64_HASH}')
sha256sums_aarch64=('${LIN_ARM64_HASH}')

package() {
  install -Dm755 creel "\${pkgdir}/usr/bin/creel"
  install -Dm644 LICENSE "\${pkgdir}/usr/share/licenses/\${pkgname}/LICENSE"
  install -Dm644 README.md "\${pkgdir}/usr/share/doc/creel/README.md"
}
EOF

cat >"$ROOT/packaging/aur/creel-bin/.SRCINFO" <<EOF
pkgbase = creel-bin
pkgname = creel-bin
pkgver = ${VERSION}
pkgrel = 1
pkgdesc = Fast, vim-driven SQL TUI for SQLite, MySQL, and PostgreSQL
url = https://github.com/${REPO}
arch = x86_64
arch = aarch64
license = MIT
provides = creel
conflicts = creel
source_x86_64 = https://github.com/${REPO}/releases/download/v${VERSION}/creel_${VERSION}_Linux_x86_64.tar.gz
sha256sums_x86_64 = ${LIN_AMD64_HASH}
source_aarch64 = https://github.com/${REPO}/releases/download/v${VERSION}/creel_${VERSION}_Linux_arm64.tar.gz
sha256sums_aarch64 = ${LIN_ARM64_HASH}
EOF

echo "Updated packaging/scoop/creel.json"
echo "Updated packaging/aur/creel-bin/{PKGBUILD,.SRCINFO}"

if [[ "$PUSH_SCOOP" -eq 1 ]]; then
  BUCKET="${SCOOP_BUCKET_DIR:-}"
  if [[ -z "$BUCKET" ]]; then
    echo "--push-scoop needs SCOOP_BUCKET_DIR pointing at a clone of rsiota/scoop-creel" >&2
    exit 1
  fi
  cp "$ROOT/packaging/scoop/creel.json" "$BUCKET/creel.json"
  echo "Copied creel.json → $BUCKET/creel.json"
  echo "Commit and push that repo to publish the Scoop bucket update."
fi
