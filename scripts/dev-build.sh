#!/usr/bin/env bash
# =============================================================================
# dev-build.sh — Local dev rebuild + stable re-sign + open.
#
# Use this instead of plain `wails build` while iterating, so that every
# rebuild uses the same self-signed dev certificate (created by
# create-dev-cert.sh). This keeps macOS TCC from re-prompting for Screen
# Recording / Accessibility permissions after every code change.
#
# Workflow:
#   ./scripts/create-dev-cert.sh   # one time
#   ./scripts/dev-build.sh         # every rebuild
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

CERT_CN="${CERT_CN:-SnapGo Dev Cert}"

# 1. Make sure wails is on PATH (works whether or not the user added it).
_GOBINPATH=$(go env GOBIN)
if [ "${_GOBINPATH}X" == "X" ]; then
    _GOBINPATH=$(go env GOPATH)/bin
fi
export PATH="${_GOBINPATH}:${PATH}"

# 2. Make sure the dev cert exists. If not, abort with a clear pointer.
if ! security find-identity -v -p codesigning | grep -q "${CERT_CN}"; then
  echo "[dev-build.sh] ERROR: code-signing identity '${CERT_CN}' not found." >&2
  echo "  Run ./scripts/create-dev-cert.sh first." >&2
  exit 1
fi

# 3. Stop any running instance so the rebuild is not blocked by file locks
#    and so the next launch uses the fresh binary.
pkill -f SnapGo 2>/dev/null || true
sleep 0.4

# 4. Build via Wails. We deliberately keep the default ad-hoc-style packaging
#    and re-sign right after, since `wails build` does not expose a
#    --identity flag in v2.
ARCH="${ARCH:-arm64}"
echo "[dev-build.sh] building darwin/${ARCH}..."
wails build -platform "darwin/${ARCH}" -clean

# 5. Re-sign with the stable dev cert. We invoke sign.sh through env var
#    DEVELOPER_ID_APPLICATION which it accepts as the identity.
APP_PATH="${ROOT_DIR}/build/bin/SnapGo.app"
echo "[dev-build.sh] re-signing ${APP_PATH} with '${CERT_CN}'..."
DEVELOPER_ID_APPLICATION="${CERT_CN}" SIGN_MODE=developer-id \
  "${SCRIPT_DIR}/sign.sh" >/dev/null

# 6. Sync to /Applications and launch from there.
#    Why /Applications: macOS TCC tracks an app by its code-signing identity
#    AND its on-disk path. Running from a stable absolute path makes the
#    privacy grants stick. We deliberately do NOT launch from build/bin to
#    avoid creating a second TCC entry every time the build dir moves.
INSTALL_PATH="/Applications/SnapGo.app"
echo "[dev-build.sh] syncing to ${INSTALL_PATH}..."
rm -rf "${INSTALL_PATH}"
cp -R "${APP_PATH}" "${INSTALL_PATH}"

echo "[dev-build.sh] opening app..."
open "${INSTALL_PATH}"

echo "[dev-build.sh] done. If TCC still re-prompts, run:"
echo "    tccutil reset ScreenCapture io.snapgo.app"
echo "    tccutil reset Accessibility io.snapgo.app"
