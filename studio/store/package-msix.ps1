param(
  [Parameter(Mandatory = $true)]
  [string]$ExecutablePath,

  [Parameter(Mandatory = $true)]
  [string]$OutputPath,

  [Parameter(Mandatory = $true)]
  [string]$IdentityName,

  [Parameter(Mandatory = $true)]
  [string]$Publisher,

  [string]$Version = "1.0.0.0",
  [string]$PublisherDisplayName = "Rizzbase",
  [string]$LogoPath = "../../public/favicon-512.png"
)

$ErrorActionPreference = "Stop"

$executable = (Resolve-Path $ExecutablePath).Path
$logo = (Resolve-Path (Join-Path $PSScriptRoot $LogoPath)).Path
$output = [System.IO.Path]::GetFullPath($OutputPath)

if ($IdentityName -notmatch "^[A-Za-z0-9.]{3,50}$") {
  throw "IdentityName must be 3-50 characters containing only letters, numbers, and periods."
}
if ([string]::IsNullOrWhiteSpace($Publisher)) {
  throw "Publisher must match the publisher identity shown in Partner Center."
}
if ($Version -notmatch "^\d+\.\d+\.\d+\.0$") {
  throw "Store package versions must have four numeric parts and end in .0."
}
foreach ($part in $Version.Split(".")) {
  if ([int]$part -gt 65535) {
    throw "Each package version component must be 65535 or lower."
  }
}
if ([int]($Version.Split(".")[0]) -eq 0) {
  throw "The first package version component must be greater than zero."
}

$makeAppx = Get-Command makeappx.exe -ErrorAction SilentlyContinue
if (-not $makeAppx) {
  $windowsKits = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"
  $makeAppx = Get-ChildItem $windowsKits -Filter "makeappx.exe" -File -Recurse |
    Where-Object { $_.FullName -match "\\x64\\makeappx\.exe$" } |
    Sort-Object FullName -Descending |
    Select-Object -First 1
}
if (-not $makeAppx) {
  throw "makeappx.exe was not found. Install the Windows 10/11 SDK."
}
$makeAppxPath = if ($makeAppx -is [System.Management.Automation.CommandInfo]) {
  $makeAppx.Source
} else {
  $makeAppx.FullName
}

$tempRoot = [System.IO.Path]::GetFullPath($env:TEMP)
$staging = Join-Path $tempRoot "rizzbase-studio-msix-$PID"
if (-not $staging.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
  throw "Refusing to create package staging outside the temporary directory."
}
if (Test-Path $staging) {
  Remove-Item -LiteralPath $staging -Recurse -Force
}
New-Item -ItemType Directory -Path $staging -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $staging "Assets") -Force | Out-Null

try {
  Copy-Item -LiteralPath $executable -Destination (Join-Path $staging "Rizzbase Studio.exe")

  Add-Type -AssemblyName System.Drawing
  $sourceImage = [System.Drawing.Image]::FromFile($logo)
  try {
    foreach ($asset in @(
      @{ Name = "StoreLogo.png"; Size = 50 },
      @{ Name = "Square44x44Logo.png"; Size = 44 },
      @{ Name = "Square150x150Logo.png"; Size = 150 }
    )) {
      $bitmap = [System.Drawing.Bitmap]::new($asset.Size, $asset.Size)
      $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
      try {
        $graphics.Clear([System.Drawing.Color]::FromArgb(11, 15, 14))
        $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $graphics.DrawImage($sourceImage, 0, 0, $asset.Size, $asset.Size)
        $assetPath = Join-Path $staging "Assets/$($asset.Name)"
        $bitmap.Save($assetPath, [System.Drawing.Imaging.ImageFormat]::Png)
      } finally {
        $graphics.Dispose()
        $bitmap.Dispose()
      }
    }
  } finally {
    $sourceImage.Dispose()
  }

  $xmlIdentity = [System.Security.SecurityElement]::Escape($IdentityName)
  $xmlPublisher = [System.Security.SecurityElement]::Escape($Publisher)
  $xmlPublisherDisplayName = [System.Security.SecurityElement]::Escape($PublisherDisplayName)
  $manifest = @"
<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
         xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
         xmlns:desktop="http://schemas.microsoft.com/appx/manifest/desktop/windows10"
         xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"
         IgnorableNamespaces="uap desktop rescap">
  <Identity Name="$xmlIdentity" Publisher="$xmlPublisher" Version="$Version" ProcessorArchitecture="x64" />
  <Properties>
    <DisplayName>Rizzbase Studio</DisplayName>
    <PublisherDisplayName>$xmlPublisherDisplayName</PublisherDisplayName>
    <Logo>Assets/StoreLogo.png</Logo>
  </Properties>
  <Dependencies>
    <TargetDeviceFamily Name="Windows.Desktop" MinVersion="10.0.19041.0" MaxVersionTested="10.0.26100.0" />
  </Dependencies>
  <Resources>
    <Resource Language="en-us" />
  </Resources>
  <Applications>
    <Application Id="RizzbaseStudio" Executable="Rizzbase Studio.exe" EntryPoint="Windows.FullTrustApplication">
      <uap:VisualElements DisplayName="Rizzbase Studio" Description="Local-first workspace for encrypted Rizzbase Projects" BackgroundColor="#0B0F0E" Square150x150Logo="Assets/Square150x150Logo.png" Square44x44Logo="Assets/Square44x44Logo.png" />
      <Extensions>
        <desktop:Extension Category="windows.fullTrustProcess" Executable="Rizzbase Studio.exe" EntryPoint="Windows.FullTrustApplication">
          <desktop:FullTrustProcess />
        </desktop:Extension>
      </Extensions>
    </Application>
  </Applications>
  <Capabilities>
    <rescap:Capability Name="runFullTrust" />
  </Capabilities>
</Package>
"@

  [System.IO.File]::WriteAllText(
    (Join-Path $staging "AppxManifest.xml"),
    $manifest,
    [System.Text.UTF8Encoding]::new($false)
  )

  $outputDirectory = Split-Path -Parent $output
  if (-not (Test-Path $outputDirectory)) {
    New-Item -ItemType Directory -Path $outputDirectory -Force | Out-Null
  }
  & $makeAppxPath pack /d $staging /p $output /o
  if ($LASTEXITCODE -ne 0) {
    throw "makeappx pack failed with exit code $LASTEXITCODE."
  }

  Write-Output "Created unsigned Microsoft Store package: $output"
} finally {
  if (Test-Path $staging) {
    Remove-Item -LiteralPath $staging -Recurse -Force
  }
}
