#!/usr/bin/env bash
# =============================================================================
# notarize.sh — Submit the signed DMG to Apple Notary Service and staple the
#               returned ticket so users get a green Gatekeeper check offline.
#
# Design rationale:
# - We use `xcrun notarytool` (Xcode 13+). The legacy `altool` is deprecated.
# - Two credential modes are supported, in order of preference:
#     1. KEYCHAIN_PROFILE  — recommended, created once via:
#          xcrun notarytool store-credentials "snapgo-notary" \
#               --apple-id "<APPLE_ID>" \
#               --team-id  "<TEAMID>" \
#               --password "<APP_SPECIFIC_PASSWORD>"
#     2. APPLE_ID + APPLE_TEAM_ID + APPLE_APP_SPECIFIC_PASSWORD env vars.
# - After successful notarization we run `xcrun stapler staple` so the app/dmg
#   is verifiable without an internet connection on the user's Mac.
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

DMG_PATH="${DMG_PATH:-}"
if [[ -z "${DMG_PATH}" ]]; then
  # Auto-detect the most recent DMG produced by make-dmg.sh
  DMG_PATH="$(ls -1t "${ROOT_DIR}/build/bin/"SnapGo-*.dmg 2>/dev/null | head -1 || true)"
fi
if [[ -z "${DMG_PATH}" || ! -f "${DMG_PATH}" ]]; then
  echo "[notarize.sh] ERROR: DMG not found. Set DMG_PATH or run make-dmg.sh first." >&2
  exit 1
fi

# -------- Build the credential argv array --------
# Using array form keeps spaces in passwords / paths safe.
NOTARY_ARGS=()
if [[ -n "${KEYCHAIN_PROFILE:-}" ]]; then
  NOTARY_ARGS+=(--keychain-profile "${KEYCHAIN_PROFILE}")
elif [[ -n "${APPLE_ID:-}" && -n "${APPLE_TEAM_ID:-}" && -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ]]; then
  NOTARY_ARGS+=(--apple-id "${APPLE_ID}"
                --team-id  "${APPLE_TEAM_ID}"
                --password "${APPLE_APP_SPECIFIC_PASSWORD}")
else
  cat >&2 <<EOF
[notarize.sh] ERROR: notary credentials not configured.

Option A (recommended) — store once in keychain, then export KEYCHAIN_PROFILE:
    xcrun notarytool store-credentials "snapgo-notary" \\
         --apple-id  "you@example.com" \\
         --team-id   "ABCDE12345" \\
         --password  "<app-specific-password>"
    export KEYCHAIN_PROFILE=snapgo-notary

Option B — export env vars directly:
    export APPLE_ID="you@example.com"
    export APPLE_TEAM_ID="ABCDE12345"
    export APPLE_APP_SPECIFIC_PASSWORD="abcd-efgh-ijkl-mnop"
EOF
  exit 1
fi

echo "[notarize.sh] submitting: ${DMG_PATH}"
xcrun notarytool submit "${DMG_PATH}" "${NOTARY_ARGS[@]}" --wait

echo "[notarize.sh] stapling ticket..."
xcrun stapler staple "${DMG_PATH}"
xcrun stapler validate "${DMG_PATH}"

# Also staple the .app inside, if we still have it on disk — this ensures the
# offline Gatekeeper check works even if the user copies the app out of the DMG.
APP_PATH="${ROOT_DIR}/build/bin/SnapGo.app"
if [[ -d "${APP_PATH}" ]]; then
  echo "[notarize.sh] stapling app bundle..."
  xcrun stapler staple "${APP_PATH}" || true
fi

echo "[notarize.sh] OK -> ${DMG_PATH}"
