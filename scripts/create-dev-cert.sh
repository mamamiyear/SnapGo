#!/usr/bin/env bash
# =============================================================================
# create-dev-cert.sh — Create a stable, self-signed code-signing certificate.
#
# Why this exists:
#   macOS's privacy database (TCC) tracks app identity primarily by the
#   code-signing cdhash (for ad-hoc) or the certificate (for properly signed
#   apps). Wails default ad-hoc signing produces a new cdhash on every build,
#   so TCC keeps re-prompting for Screen Recording / Accessibility permission.
#
#   By creating ONE self-signed certificate locally and reusing it for every
#   build, the signing identity never changes, so TCC remembers the grant.
#
# Idempotent: if the cert already exists with the configured CN, this script
# is a no-op and exits 0.
# =============================================================================
set -euo pipefail

CERT_CN="${CERT_CN:-SnapGo Dev Cert}"
KEYCHAIN="${KEYCHAIN:-login.keychain-db}"

# 1. Skip ONLY if a usable code-signing identity exists. We deliberately do
#    not skip on the mere presence of a certificate object, because a partial
#    import (e.g. wrong PKCS#12 password) leaves the cert behind without a
#    private key, which makes codesign fail later in confusing ways.
if security find-identity -v -p codesigning 2>/dev/null | grep -q "\"${CERT_CN}\""; then
  echo "[create-dev-cert.sh] usable identity '${CERT_CN}' already present — nothing to do."
  exit 0
fi

# 1b. Clean any stale certificate-only entry from a previous failed import,
#     so the new import does not collide on duplicate CN.
if security find-certificate -c "${CERT_CN}" "${KEYCHAIN}" >/dev/null 2>&1; then
  echo "[create-dev-cert.sh] removing stale (key-less) cert entry from previous run..."
  # Loop because there may be multiple stale copies.
  while security find-certificate -c "${CERT_CN}" "${KEYCHAIN}" >/dev/null 2>&1; do
    security delete-certificate -c "${CERT_CN}" "${KEYCHAIN}" >/dev/null 2>&1 || break
  done
fi

# 2. Generate a self-signed code-signing cert via openssl + import via security.
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

KEY="${WORK}/dev.key"
CSR="${WORK}/dev.csr"
CRT="${WORK}/dev.crt"
P12="${WORK}/dev.p12"
EXTFILE="${WORK}/ext.cnf"

# X509 v3 extension file enabling "Code Signing" extended key usage (EKU).
cat > "${EXTFILE}" <<'EOF'
[v3_ext]
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
basicConstraints = CA:false
EOF

echo "[create-dev-cert.sh] generating private key..."
openssl genrsa -out "${KEY}" 2048 >/dev/null 2>&1

echo "[create-dev-cert.sh] generating CSR..."
openssl req -new -key "${KEY}" -out "${CSR}" -subj "/CN=${CERT_CN}/O=SnapGo Dev/C=US" >/dev/null 2>&1

echo "[create-dev-cert.sh] self-signing certificate (10 years)..."
openssl x509 -req -in "${CSR}" -signkey "${KEY}" -out "${CRT}" \
  -days 3650 -extfile "${EXTFILE}" -extensions v3_ext >/dev/null 2>&1

echo "[create-dev-cert.sh] bundling into PKCS#12..."
# `-legacy` is REQUIRED on OpenSSL 3+: macOS's `security` tool can only
# verify the legacy PKCS#12 MAC (SHA-1 / 3DES). Without this flag the import
# fails with "MAC verification failed during PKCS12 import".
# Use a non-empty passphrase to avoid edge cases where empty pass triggers
# different MAC behaviour on some openssl builds.
P12_PASS="snapgo"
openssl pkcs12 -export -legacy \
  -inkey "${KEY}" -in "${CRT}" -out "${P12}" \
  -name "${CERT_CN}" -passout "pass:${P12_PASS}" >/dev/null 2>&1

echo "[create-dev-cert.sh] importing into ${KEYCHAIN} (you may be prompted for your login password)..."
security import "${P12}" -k "${KEYCHAIN}" -P "${P12_PASS}" -T /usr/bin/codesign

# 3. Mark the cert as trusted for code signing. This requires admin.
#    We use `security add-trusted-cert` with policy `codeSign`.
echo "[create-dev-cert.sh] adding trust setting (admin password may be required)..."
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain \
  -p codeSign "${CRT}" || {
    echo "[create-dev-cert.sh] WARN: trust setting could not be added. The cert"
    echo "  is still importable for codesign use, but TCC may still re-prompt"
    echo "  unless the cert is trusted. Re-run as admin to fix."
}

echo "[create-dev-cert.sh] done. Verify with:"
echo "    security find-identity -v -p codesigning"
echo "You should see: '${CERT_CN}'"
