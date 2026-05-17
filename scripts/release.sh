#!/usr/bin/env bash
# =============================================================================
# release.sh — One-shot build → sign → DMG → notarize → staple pipeline.
#
# Design rationale:
# - Each stage is delegated to a dedicated single-purpose script. This keeps the
#   pipeline composable: a developer can re-run any individual stage without
#   redoing the whole release.
# - Notarization is OPT-IN. Setting NOTARIZE=1 (or providing notary credentials)
#   triggers it; otherwise we stop after producing a signed DMG, which is enough
#   for internal distribution and CI smoke tests.
# - ARCH defaults to the host arch so a developer on Apple Silicon gets an
#   arm64 build. Set ARCH=universal to ship a fat binary.
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

ARCH="${ARCH:-$(uname -m)}"
case "${ARCH}" in
  arm64)      WAILS_PLATFORM="darwin/arm64" ;;
  x86_64)     WAILS_PLATFORM="darwin/amd64" ;;
  universal)  WAILS_PLATFORM="darwin/universal" ;;
  *) echo "[release.sh] ERROR: unsupported ARCH=${ARCH}" >&2; exit 1 ;;
esac

echo "================================================================"
echo " SnapGo release pipeline"
echo "   arch     : ${ARCH}  (${WAILS_PLATFORM})"
echo "   sign     : ${SIGN_MODE:-auto}"
echo "   notarize : ${NOTARIZE:-0}"
echo "================================================================"

# ---- 1. Build ----
echo ""
echo "[release.sh] (1/4) wails build"
( cd "${ROOT_DIR}" && wails build -platform "${WAILS_PLATFORM}" -clean )

# ---- 2. Sign .app ----
echo ""
echo "[release.sh] (2/4) sign app"
"${SCRIPT_DIR}/sign.sh"

# ---- 3. DMG ----
echo ""
echo "[release.sh] (3/4) make DMG"
ARCH="${ARCH}" "${SCRIPT_DIR}/make-dmg.sh"

# ---- 4. Notarize (optional) ----
echo ""
if [[ "${NOTARIZE:-0}" == "1" || -n "${KEYCHAIN_PROFILE:-}" \
   || ( -n "${APPLE_ID:-}" && -n "${APPLE_TEAM_ID:-}" \
        && -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ) ]]; then
  echo "[release.sh] (4/4) notarize"
  "${SCRIPT_DIR}/notarize.sh"
else
  echo "[release.sh] (4/4) notarize SKIPPED (set NOTARIZE=1 with credentials to enable)"
fi

echo ""
echo "[release.sh] DONE."
ls -lh "${ROOT_DIR}/build/bin/"SnapGo-*.dmg 2>/dev/null || true
