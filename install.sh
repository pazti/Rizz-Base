#!/usr/bin/env bash
set -euo pipefail

version="${RIZZBASE_VERSION:-1.1.0}"
install_dir="${RIZZBASE_INSTALL_DIR:-$HOME/.rizzbase/bin}"
release_base="${RIZZBASE_RELEASE_BASE_URL:-https://github.com/pazti/Rizz-Base/releases/download}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "RIZZBASE_VERSION must be a version like 1.1.0." >&2
  exit 1
fi

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *)
    echo "Unsupported operating system: $(uname -s). Supported: Linux and macOS." >&2
    exit 1
    ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *)
    echo "Unsupported architecture: $(uname -m). Supported: x64 and arm64." >&2
    exit 1
    ;;
esac

asset="rizzbase-${os}-${arch}"
release="${release_base%/}/v${version}"
mkdir -p "$install_dir"
temporary_dir="$(mktemp -d "${install_dir%/}/.rizzbase-install.XXXXXX")"
cleanup() { rm -rf "$temporary_dir"; }
trap cleanup EXIT

if ! curl --fail --location --silent --show-error "$release/$asset" -o "$temporary_dir/$asset"; then
  echo "Could not download $asset for Rizzbase v$version." >&2
  exit 1
fi
if ! curl --fail --location --silent --show-error "$release/SHA256SUMS" -o "$temporary_dir/SHA256SUMS"; then
  echo "Could not download the Rizzbase checksum manifest." >&2
  exit 1
fi

expected="$(awk -v asset="$asset" '$2 == asset || $2 == "*" asset { if (++count == 1) hash = $1 } END { if (count == 1) print hash }' "$temporary_dir/SHA256SUMS")"
if [[ ! "$expected" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo "Checksum manifest has no unique valid entry for $asset." >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary_dir/$asset" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$temporary_dir/$asset" | awk '{print $1}')"
else
  echo "A SHA-256 utility (sha256sum or shasum) is required." >&2
  exit 1
fi
if [[ "${actual,,}" != "${expected,,}" ]]; then
  echo "Checksum mismatch for $asset; the existing installation was not changed." >&2
  exit 1
fi

chmod 0755 "$temporary_dir/$asset"
mv -f "$temporary_dir/$asset" "$install_dir/rizzbase"

echo "Installed Rizzbase v$version at $install_dir/rizzbase"
echo "Add it to PATH with: export PATH=\"$install_dir:\$PATH\""
echo "Verify with: $install_dir/rizzbase --version"
