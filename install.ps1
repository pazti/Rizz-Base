[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$ReleaseApiBase = if ($env:RIZZBASE_RELEASE_API_URL) { $env:RIZZBASE_RELEASE_API_URL.TrimEnd("/") } else { "https://api.github.com/repos/pazti/Rizz-Base" }
$Version = if ($env:RIZZBASE_VERSION) { $env:RIZZBASE_VERSION } else { "latest" }
$InstallDir = if ($env:RIZZBASE_INSTALL_DIR) { $env:RIZZBASE_INSTALL_DIR } else { Join-Path $HOME ".rizzbase\bin" }
$ReleaseBase = if ($env:RIZZBASE_RELEASE_BASE_URL) { $env:RIZZBASE_RELEASE_BASE_URL.TrimEnd("/") } else { "https://github.com/pazti/Rizz-Base/releases/download" }

if ($Version -eq "latest") {
  $Latest = Invoke-RestMethod -Uri "$ReleaseApiBase/releases/latest" -Headers @{ Accept = "application/vnd.github+json" }
  if (-not $Latest.tag_name) { throw "Could not determine the latest Rizzbase release." }
  $Version = $Latest.tag_name.TrimStart("v")
}
elseif ($Version.StartsWith("v")) {
  $Version = $Version.Substring(1)
}

if ($Version -notmatch '^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$') {
  throw "RIZZBASE_VERSION must be a stable version like 1.1.0."
}
if (-not [Environment]::Is64BitOperatingSystem) {
  throw "Unsupported architecture. Rizzbase supports Windows x64 and arm64."
}
$Architecture = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq [System.Runtime.InteropServices.Architecture]::Arm64) { "arm64" } else { "amd64" }
$Asset = "rizzbase-windows-$Architecture.exe"
$Release = "$ReleaseBase/v$Version"
$TemporaryDir = Join-Path $InstallDir ".rizzbase-install-$([guid]::NewGuid().ToString('N'))"

try {
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  New-Item -ItemType Directory -Path $TemporaryDir | Out-Null
  $BinaryTemporary = Join-Path $TemporaryDir $Asset
  $Manifest = Join-Path $TemporaryDir "SHA256SUMS"
  Invoke-WebRequest -Uri "$Release/$Asset" -OutFile $BinaryTemporary
  Invoke-WebRequest -Uri "$Release/SHA256SUMS" -OutFile $Manifest

  $ChecksumEntries = @()
  foreach ($Line in Get-Content -LiteralPath $Manifest) {
    $ChecksumMatch = [regex]::Match($Line, '^([a-fA-F0-9]{64})\s+\*?(\S+)$')
    if ($ChecksumMatch.Success -and $ChecksumMatch.Groups[2].Value -ceq $Asset) {
      $ChecksumEntries += $ChecksumMatch.Groups[1].Value
    }
  }
  if ($ChecksumEntries.Count -ne 1) {
    throw "Checksum manifest has no unique entry for $Asset."
  }
  $Expected = $ChecksumEntries[0].ToLowerInvariant()
  $Actual = (Get-FileHash -LiteralPath $BinaryTemporary -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($Expected -ne $Actual) {
    throw "Checksum mismatch for $Asset; the existing installation was not changed."
  }

  $Destination = Join-Path $InstallDir "rizzbase.exe"
  if (Test-Path -LiteralPath $Destination) {
    [System.IO.File]::Replace($BinaryTemporary, $Destination, $null)
  } else {
    [System.IO.File]::Move($BinaryTemporary, $Destination)
  }
  Write-Host "Installed Rizzbase v$Version at $Destination"
  Write-Host "Add it to your user PATH with:"
  Write-Host "[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ';$InstallDir', 'User')"
  Write-Host "Verify with: `"$Destination`" --version"
}
finally {
  if (Test-Path -LiteralPath $TemporaryDir) {
    Remove-Item -LiteralPath $TemporaryDir -Recurse -Force
  }
}
