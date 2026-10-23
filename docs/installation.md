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

The installers default to release `1.1.0` and `~/.rizzbase/bin`. Set
`RIZZBASE_VERSION` and/or `RIZZBASE_INSTALL_DIR` before running the installer
to choose another version or destination. Set
`RIZZBASE_RELEASE_BASE_URL` only when using a compatible release mirror.

The Node SDK supports engine versions `>=1.1.0 <2.0.0` and project format `1`.
Engine and SDK release versions do not need to match. `rizzbase --version`
prints machine-readable JSON containing both values.

## Manual installation

Release assets are versioned and have a `SHA256SUMS` file:

```text
https://github.com/pazti/Rizz-Base/releases/download/v1.1.0/<asset>
https://github.com/pazti/Rizz-Base/releases/download/v1.1.0/SHA256SUMS
```

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

Checksums detect corruption but do not prove who published a binary. For a
stronger supply-chain policy, build the engine from source, inspect it, and
set the SDK's `binaryPath` to that executable. A source build reports `dev`;
pass a stable semver at build time to use it with the Node SDK:

```sh
cd engine
go build -ldflags="-X main.version=1.1.0" -o ~/.rizzbase/bin/rizzbase ./cmd/rizzbase
```

## SDK binary selection

The SDK resolves its executable in this order:

1. `binaryPath` in the SDK constructor.
2. `RIZZBASE_BINARY`.
3. `rizzbase` (or `rizzbase.exe` on Windows) on `PATH`.

If no executable is found, the SDK reports `BINARY_UNAVAILABLE` with the
locations it searched. It never downloads a binary.
