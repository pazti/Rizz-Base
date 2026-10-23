$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Temporary = Join-Path ([System.IO.Path]::GetTempPath()) "rizzbase-installer-test-$([guid]::NewGuid().ToString('N'))"
$ReleaseDirectory = Join-Path $Temporary "release\v1.2.3"
$InstallDirectory = Join-Path $Temporary "installed"
$Asset = "rizzbase-windows-amd64.exe"
$Server = $null
$OldValues = @{
  RIZZBASE_VERSION = $env:RIZZBASE_VERSION
  RIZZBASE_INSTALL_DIR = $env:RIZZBASE_INSTALL_DIR
  RIZZBASE_RELEASE_BASE_URL = $env:RIZZBASE_RELEASE_BASE_URL
}

try {
  New-Item -ItemType Directory -Force -Path $ReleaseDirectory | Out-Null
  $Binary = [System.Text.Encoding]::UTF8.GetBytes("verified Windows test executable")
  [System.IO.File]::WriteAllBytes((Join-Path $ReleaseDirectory $Asset), $Binary)
  $Hash = (Get-FileHash -LiteralPath (Join-Path $ReleaseDirectory $Asset) -Algorithm SHA256).Hash.ToLowerInvariant()
  Set-Content -LiteralPath (Join-Path $ReleaseDirectory "SHA256SUMS") -Value "$Hash  $Asset" -Encoding ascii

  $Listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  $Listener.Start()
  $Port = ([System.Net.IPEndPoint]$Listener.LocalEndpoint).Port
  $Listener.Stop()
  $Server = Start-Process -FilePath "python" -ArgumentList @("-m", "http.server", "$Port", "--bind", "127.0.0.1", "--directory", $Temporary) -PassThru -WindowStyle Hidden

  $Ready = $false
  for ($Attempt = 0; $Attempt -lt 30; $Attempt++) {
    try {
      Invoke-WebRequest -Uri "http://127.0.0.1:$Port/" -UseBasicParsing | Out-Null
      $Ready = $true
      break
    } catch {
      Start-Sleep -Milliseconds 200
    }
  }
  if (-not $Ready) { throw "Local installer test server did not start." }

  $env:RIZZBASE_VERSION = "1.2.3"
  $env:RIZZBASE_INSTALL_DIR = $InstallDirectory
  $env:RIZZBASE_RELEASE_BASE_URL = "http://127.0.0.1:$Port/release"
  & (Join-Path $Root "install.ps1")

  $Destination = Join-Path $InstallDirectory "rizzbase.exe"
  $InstalledHash = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($InstalledHash -ne $Hash) {
    throw "Installed Windows executable does not match the downloaded asset."
  }

  [System.IO.File]::WriteAllBytes((Join-Path $ReleaseDirectory $Asset), [System.Text.Encoding]::UTF8.GetBytes("tampered"))
  $Failed = $false
  try {
    & (Join-Path $Root "install.ps1")
  } catch {
    $Failed = $true
  }
  if (-not $Failed) { throw "Installer accepted an asset with a bad checksum." }
  $InstalledHash = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($InstalledHash -ne $Hash) {
    throw "Checksum failure changed the existing Windows installation."
  }

  Write-Host "Windows installer integration tests passed"
}
finally {
  foreach ($Name in $OldValues.Keys) {
    if ($null -eq $OldValues[$Name]) {
      Remove-Item "Env:$Name" -ErrorAction SilentlyContinue
    } else {
      Set-Item "Env:$Name" $OldValues[$Name]
    }
  }
  if ($Server -and -not $Server.HasExited) {
    Stop-Process -Id $Server.Id -Force
  }
  if (Test-Path -LiteralPath $Temporary) {
    Remove-Item -LiteralPath $Temporary -Recurse -Force
  }
}
