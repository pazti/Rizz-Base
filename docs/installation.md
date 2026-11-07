# Install the engine binary

You do **not** need npm to run Rizzbase. Install the engine yourself; the SDK
never downloads or executes a binary implicitly.

## Linux and macOS

Download the installer, inspect it, then run it. Do not pipe a remote script
directly into a shell:

```sh
curl -fLO https://raw.githubusercontent.com/pazti/Rizz-Base/main/install.sh
less install.sh
chmod +x install.sh
./install.sh
```

The installer detects Linux/macOS and x64/arm64, downloads the pinned release
asset, verifies its SHA-256 against the exact matching `SHA256SUMS` entry, then
installs it atomically at `~/.rizzbase/bin/rizzbase` with mode `0755`. It
requires `curl` and either `sha256sum` or `shasum`; it does not require npm or
`sudo`.

Add the printed PATH command to your shell profile, or run:

```sh
export PATH="$HOME/.rizzbase/bin:$PATH"
rizzbase --version
```

## Windows (PowerShell)

Download and inspect the installer before running it:

```powershell
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/pazti/Rizz-Base/main/install.ps1" -OutFile ".\install.ps1"
Get-Content .\install.ps1
.\install.ps1
```

It supports Windows x64 and arm64, verifies the exact asset checksum with
`Get-FileHash`, and atomically replaces
`%USERPROFILE%\.rizzbase\bin\rizzbase.exe` only after verification. The
installer prints the commands to add the directory to your user PATH and to
check the version.

## Pinning a version or changing the install directory

The installers default to the latest published release and install into
`~/.rizzbase/bin`. Set `RIZZBASE_VERSION` and/or `RIZZBASE_INSTALL_DIR`
before running the installer to choose another version or destination. Set
`RIZZBASE_RELEASE_BASE_URL` and `RIZZBASE_RELEASE_API_URL` only when using a
compatible release mirror or a custom API endpoint.

Both SDKs support engine versions `>=1.1.0 <2.0.0` and project format `1`.
Engine and SDK release versions do not need to match. `rizzbase --version`
prints machine-readable JSON containing both values.

## Manual installation

Release assets are versioned and have a `SHA256SUMS` file. Releases created
after the signed-release workflow is enabled will also include a Sigstore
Cosign bundle that signs the checksum manifest and binds it to the GitHub
release workflow identity. Older releases may not have a signature bundle.
For a signed release, verify that signature before trusting the checksums:

```text
https://github.com/pazti/Rizz-Base/releases/download/<tag>/<asset>
https://github.com/pazti/Rizz-Base/releases/download/<tag>/SHA256SUMS
https://github.com/pazti/Rizz-Base/releases/download/<tag>/SHA256SUMS.sigstore.json
```

Install [Cosign](https://docs.sigstore.dev/cosign/system_config/installation/)
and verify the manifest bundle. Download the files for the exact release tag
you are installing:

```sh
cosign verify-blob SHA256SUMS \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/pazti/Rizz-Base/.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Only after successful signature verification, verify the selected asset
against `SHA256SUMS` as shown below. The installers currently check the
checksum but do not verify the Sigstore signature automatically; users
requiring publisher-authenticity verification should run Cosign first.

Assets are named `rizzbase-linux-{amd64,arm64}`,
`rizzbase-darwin-{amd64,arm64}`, and
`rizzbase-windows-{amd64,arm64}.exe`. If installing manually, download the
asset using its exact release filename into the same directory as
`SHA256SUMS`, then verify only that asset's exact manifest line:

```sh
ASSET=rizzbase-linux-amd64 # choose the asset for your platform
awk -v asset="$ASSET" '$2 == asset { print }' SHA256SUMS | sha256sum -c -
```

On macOS, replace `sha256sum -c -` with `shasum -a 256 -c -`. In PowerShell,
select the exact filename and compare the resulting hash:

```powershell
$Asset = "rizzbase-windows-amd64.exe"
$Entries = foreach ($Line in Get-Content .\SHA256SUMS) {
  $Match = [regex]::Match($Line, '^([a-fA-F0-9]{64})\s+\*?(\S+)$')
  if ($Match.Success -and $Match.Groups[2].Value -ceq $Asset) { $Match.Groups[1].Value }
}
if (@($Entries).Count -ne 1) { throw "No unique checksum entry for $Asset" }
$Expected = $Entries[0].ToLowerInvariant()
$Actual = (Get-FileHash -LiteralPath ".\$Asset" -Algorithm SHA256).Hash.ToLowerInvariant()
if ($Expected -ne $Actual) { throw "Checksum mismatch" }
```

Only after verification, rename/move the asset into place. Do not verify a
manifest line that names a different local filename.

The release workflow builds each platform binary twice using Go 1.27.1 with
trimmed paths and VCS metadata disabled; the second build must match byte for
byte before publication. It signs `SHA256SUMS` using keyless Sigstore
Cosign/GitHub OIDC. The signature verifies the release manifest and its
workflow identity; the manifest in turn hashes all platform binaries.

For an independent build policy, build the engine from source, inspect it, and
set the SDK's `binaryPath` (Node.js) or `binary_path` (Python) to that
executable. A source build reports `dev`;
pass a stable semver at build time to use it with the Node SDK:

```sh
cd engine
go build -ldflags="-X main.version=1.1.0" -o ~/.rizzbase/bin/rizzbase ./cmd/rizzbase
```

## SDK binary selection

Both SDKs resolve the engine in this order:

1. `binaryPath` (Node.js) or `binary_path` (Python) in the SDK constructor.
2. `RIZZBASE_BINARY`.
3. `rizzbase` (or `rizzbase.exe` on Windows) on `PATH`.

If no executable is found, the SDK reports `BINARY_UNAVAILABLE` with the
locations it searched. Neither SDK downloads a binary.
