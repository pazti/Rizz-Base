# Microsoft Store packaging

Wails v2.17.1 builds the Windows application executable; it does not generate
an MSIX package. `package-msix.ps1` uses the Windows SDK's MakeAppx tool to
package that executable and the Rizzbase Store icons as an MSIX. Studio imports
`rizzbase/engine/core` directly, so Go compiles the engine into the Studio
executable. The package does not need a separate engine in `~/.rizzbase/bin`.

## Create an MSIX

Build Studio on Windows with Wails, then run this from the repository root.
The identity and publisher must match the app identity assigned in Partner
Center before submitting the package:

```powershell
cd studio
wails build -platform windows/amd64 -webview2 download
cd ..

pwsh -File studio/store/package-msix.ps1 `
  -ExecutablePath "studio/build/bin/Rizzbase Studio.exe" `
  -OutputPath "studio/build/bin/RizzbaseStudio.msix" `
  -IdentityName "<Package/Identity Name from Partner Center>" `
  -Publisher "<Publisher from Partner Center>" `
  -PublisherDisplayName "<Publisher display name from Partner Center>" `
  -Version "1.0.0.0"
```

Use the next higher Store package version for each submission; the fourth
version component must remain `0`. The generated package is unsigned. Microsoft
Store signs accepted MSIX packages during publishing. For local testing, sign
the package with a development certificate trusted on the test machine.

The `Rizzbase Studio Windows Installer` GitHub Actions workflow builds and
tests both the existing NSIS installer and a test-identity MSIX on a clean
Windows runner, including Windows App Certification Kit validation. Its MSIX
uses a local test identity and is **not** the package to submit to Partner
Center. Run the workflow manually with the reserved identity, publisher,
publisher display name, and version to produce a submission-ready unsigned
MSIX.

## Capabilities and Store assets

The package declares only `runFullTrust`, which is required to run the Wails
desktop executable. Studio opens and saves Projects and backups through
user-initiated file dialogs; it does not request `broadFileSystemAccess` or
`internetClient`, and does not start a helper engine or network service. In
certification notes, explain that full trust is needed for the native Wails
window and embedded Go engine, while project files remain on-device at paths
chosen by the user.

The Windows executable uses Wails' WebView2 `download` strategy. If the
runtime is missing or outdated, Wails asks before starting Microsoft's setup
flow and reports setup failures instead of silently falling back to another
browser.

The package's square logos are generated from the official dark Rizzbase
favicon mark with its teal accent. Wails also takes its executable and
installer icon from this brand asset. Use the brand wordmark and teal-on-dark
palette in Store listing screenshots; capture the actual Studio UI with a
throwaway Project containing no personal data.

## Partner Center checklist

1. Register and complete identity verification for a Microsoft Partner Center
   developer account.
2. Reserve **Rizzbase Studio** (if available) and complete the Store listing.
3. Copy the exact package identity name, publisher, and publisher display name
   from the app's identity details into the workflow inputs.
4. Add `https://rizzbase.tech/privacy/` as the Store privacy-policy URL.
5. Capture real Studio screenshots with a throwaway Project, using the official
   wordmark and teal-on-dark brand styling. Do not include personal data.
6. Upload the unsigned MSIX generated with the reserved identity, provide the
   screenshots and required disclosures, and submit for certification.

Account registration, identity verification, name reservation, and Store
submission require the account owner's actions in Partner Center.
