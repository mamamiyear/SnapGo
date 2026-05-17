#!/usr/bin/env bash
# =============================================================================
# make-dmg.sh — Build a distributable .dmg for SnapGo.app
#
# Design rationale:
# - Prefer `create-dmg` (https://github.com/create-dmg/create-dmg) when available
#   because it produces a polished installer with /Applications drag target,
#   custom background and proper icon layout.
# - Fall back to a plain `hdiutil create` UDZO image so the script always works
#   on a fresh machine even without homebrew. The fallback DMG still shows the
#   app + a symlink to /Applications so users can drag-install.
# - Output is always written to build/bin/SnapGo-<version>-<arch>.dmg so it
#   does not collide with the .app bundle.
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
APP_PATH="${APP_PATH:-${ROOT_DIR}/build/bin/SnapGo.app}"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/build/bin}"

if [[ ! -d "${APP_PATH}" ]]; then
  echo "[make-dmg.sh] ERROR: ${APP_PATH} not found." >&2
  exit 1
fi

# -------- Version / arch derivation --------
VERSION="${VERSION:-$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' \
                       "${APP_PATH}/Contents/Info.plist" 2>/dev/null || echo "0.1.0")}"
ARCH="${ARCH:-$(uname -m)}"   # arm64 or x86_64
DMG_NAME="SnapGo-${VERSION}-${ARCH}.dmg"
DMG_PATH="${OUT_DIR}/${DMG_NAME}"
VOLNAME="SnapGo ${VERSION}"

mkdir -p "${OUT_DIR}"
rm -f "${DMG_PATH}"

# -------- Path 1: create-dmg (pretty) --------
if command -v create-dmg >/dev/null 2>&1; then
  echo "[make-dmg.sh] using create-dmg"
  # NOTE: create-dmg refuses to run when the output exists; we already removed it.
  create-dmg \
      --volname "${VOLNAME}" \
      --window-pos 200 120 \
      --window-size 600 360 \
      --icon-size 100 \
      --icon "SnapGo.app" 160 180 \
      --hide-extension "SnapGo.app" \
      --app-drop-link 440 180 \
      --no-internet-enable \
      "${DMG_PATH}" \
      "${APP_PATH}"
else
  # -------- Path 2: hdiutil fallback (always works) --------
  echo "[make-dmg.sh] create-dmg not found, using hdiutil fallback"
  STAGING="$(mktemp -d)"
  trap 'rm -rf "${STAGING}"' EXIT

  # Copy the .app and add an /Applications symlink so users can drag-install.
  cp -R "${APP_PATH}" "${STAGING}/"
  ln -s /Applications "${STAGING}/Applications"

  hdiutil create \
      -volname "${VOLNAME}" \
      -srcfolder "${STAGING}" \
      -ov -format UDZO \
      "${DMG_PATH}"
fi

# -------- Sign the DMG itself if a Developer ID is available --------
# Notarization requires the DMG be signed too.
if [[ -n "${DEVELOPER_ID_APPLICATION:-}" ]]; then
  echo "[make-dmg.sh] signing DMG with: ${DEVELOPER_ID_APPLICATION}"
  codesign --force --sign "${DEVELOPER_ID_APPLICATION}" --timestamp "${DMG_PATH}"
elif security find-identity -v -p codesigning 2>/dev/null \
        | grep -q "Developer ID Application"; then
  IDENT=$(security find-identity -v -p codesigning \
            | awk -F'"' '/Developer ID Application/ {print $2; exit}')
  echo "[make-dmg.sh] signing DMG with auto-detected identity: ${IDENT}"
  codesign --force --sign "${IDENT}" --timestamp "${DMG_PATH}"
else
  echo "[make-dmg.sh] no Developer ID identity, DMG left unsigned (ad-hoc build)."
fi

echo "[make-dmg.sh] OK -> ${DMG_PATH}"
