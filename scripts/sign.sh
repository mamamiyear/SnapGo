#!/usr/bin/env bash
# =============================================================================
# sign.sh — Sign SnapGo.app with Hardened Runtime.
#
# Design rationale:
# - Two modes are supported, controlled purely by env vars (no CLI args), so
#   the script stays idempotent and easy to wire into CI:
#     1. "developer-id"  : real Developer ID Application signing for distribution
#                          (requires DEVELOPER_ID_APPLICATION env or default
#                           lookup via `security find-identity`).
#     2. "ad-hoc"        : codesign with `-` identity. The bundle works on the
#                          building machine after `xattr -dr com.apple.quarantine`,
#                          but cannot be notarized.
# - We always sign with --options=runtime so the bundle is notarization-ready.
# - We sign nested binaries first, then the .app bundle (Apple's required order).
# =============================================================================
set -euo pipefail

# -------- Paths --------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
APP_PATH="${APP_PATH:-${ROOT_DIR}/build/bin/SnapGo.app}"
ENTITLEMENTS="${ENTITLEMENTS:-${ROOT_DIR}/build/darwin/entitlements.plist}"

# -------- Mode resolution --------
# SIGN_MODE=developer-id | ad-hoc (default: ad-hoc when no identity is found)
SIGN_MODE="${SIGN_MODE:-auto}"
SIGN_IDENTITY=""

resolve_identity() {
  if [[ -n "${DEVELOPER_ID_APPLICATION:-}" ]]; then
    SIGN_IDENTITY="${DEVELOPER_ID_APPLICATION}"
    SIGN_MODE="developer-id"
    return
  fi
  # Try to auto-discover the first Developer ID Application identity.
  local found
  found=$(security find-identity -v -p codesigning 2>/dev/null \
            | awk -F'"' '/Developer ID Application/ {print $2; exit}')
  if [[ -n "${found}" ]]; then
    SIGN_IDENTITY="${found}"
    SIGN_MODE="developer-id"
  else
    SIGN_IDENTITY="-"
    SIGN_MODE="ad-hoc"
  fi
}

if [[ "${SIGN_MODE}" == "auto" ]]; then
  resolve_identity
elif [[ "${SIGN_MODE}" == "developer-id" ]]; then
  if [[ -z "${DEVELOPER_ID_APPLICATION:-}" ]]; then
    resolve_identity
    if [[ "${SIGN_MODE}" != "developer-id" ]]; then
      echo "[sign.sh] ERROR: SIGN_MODE=developer-id but no Developer ID Application identity found." >&2
      echo "          Set DEVELOPER_ID_APPLICATION='Developer ID Application: NAME (TEAMID)'" >&2
      exit 1
    fi
  else
    SIGN_IDENTITY="${DEVELOPER_ID_APPLICATION}"
  fi
elif [[ "${SIGN_MODE}" == "ad-hoc" ]]; then
  SIGN_IDENTITY="-"
else
  echo "[sign.sh] ERROR: invalid SIGN_MODE='${SIGN_MODE}'" >&2
  exit 1
fi

# -------- Sanity check --------
if [[ ! -d "${APP_PATH}" ]]; then
  echo "[sign.sh] ERROR: app bundle not found at ${APP_PATH}" >&2
  echo "          Run 'wails build -platform darwin/arm64 -clean' first." >&2
  exit 1
fi
if [[ ! -f "${ENTITLEMENTS}" ]]; then
  echo "[sign.sh] ERROR: entitlements not found at ${ENTITLEMENTS}" >&2
  exit 1
fi

echo "[sign.sh] mode      = ${SIGN_MODE}"
echo "[sign.sh] identity  = ${SIGN_IDENTITY}"
echo "[sign.sh] app       = ${APP_PATH}"
echo "[sign.sh] ents      = ${ENTITLEMENTS}"

# -------- Decide whether to request RFC3161 timestamp --------
# Apple's timestamp server only accepts signatures from Developer ID-issued
# certificates. For ad-hoc or local self-signed certs the timestamp call
# either fails or attaches noise that makes re-signing nondeterministic
# (which in turn breaks TCC's "is this the same app?" comparison).
TIMESTAMP_FLAG="--timestamp=none"
if [[ "${SIGN_MODE}" == "developer-id" && "${SIGN_IDENTITY}" == "Developer ID Application:"* ]]; then
  TIMESTAMP_FLAG="--timestamp"
fi
echo "[sign.sh] timestamp = ${TIMESTAMP_FLAG}"

# -------- Strip stale signatures so re-signing is deterministic --------
codesign --remove-signature "${APP_PATH}" 2>/dev/null || true

# -------- Sign nested executables first (Apple required ordering) --------
# Find any embedded Mach-O binaries / frameworks / dylibs.
NESTED_PATHS=()
while IFS= read -r path; do
  NESTED_PATHS+=("$path")
done < <(find "${APP_PATH}/Contents" \
            \( -name "*.dylib" -o -name "*.framework" -o -path "*/Frameworks/*" \) \
            -not -path "*/_CodeSignature/*" 2>/dev/null || true)

for nested in "${NESTED_PATHS[@]:-}"; do
  [[ -z "${nested}" ]] && continue
  echo "[sign.sh] signing nested: ${nested}"
  codesign --force ${TIMESTAMP_FLAG} --options=runtime \
           --sign "${SIGN_IDENTITY}" \
           "${nested}"
done

# -------- Sign the main app bundle --------
echo "[sign.sh] signing app bundle..."
codesign --force ${TIMESTAMP_FLAG} --options=runtime \
         --entitlements "${ENTITLEMENTS}" \
         --sign "${SIGN_IDENTITY}" \
         "${APP_PATH}"

# -------- Verify --------
echo "[sign.sh] verifying signature..."
codesign --verify --deep --strict --verbose=2 "${APP_PATH}"

if [[ "${SIGN_MODE}" == "developer-id" ]]; then
  # Gatekeeper assessment will pass only after notarization + stapling,
  # but we can pre-flight the spctl evaluation now.
  echo "[sign.sh] spctl assessment (will likely say 'rejected' until notarized)..."
  spctl --assess --type execute --verbose=4 "${APP_PATH}" || true
fi

echo "[sign.sh] OK."
