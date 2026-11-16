# Rizzbase Studio

Rizzbase Studio is a local desktop workspace for encrypted `.rizz` Projects.
It uses Wails v2 with the existing Go engine directly; it does not start a
hosted service or make database requests over the network.

The first Studio release supports creating and opening Projects, restoring a
decrypted JSON backup as a new encrypted Project, creating and deleting
Lobbies, browsing and editing Yaps as JSON, and exporting a decrypted JSON
backup. Restores never overwrite an existing Project and require a new
passphrase. Before restoring, Studio validates the backup and previews its
Project name, creation date, Lobby count, and Yap count. The passphrase is kept
in app memory while a Project is open and is discarded when the Project is
closed or Studio exits. JSON backups are plaintext and must be protected.

## Development

Prerequisites:

- Go 1.26+
- Node.js 20+
- pnpm
- Wails CLI v2.17.1
- The Wails platform dependencies for your OS (Ubuntu 24.04 requires
  `libgtk-3-dev`, `libwebkit2gtk-4.1-dev`, and `libsoup-3.0-dev`)

From the repository root, install the frontend packages and Wails CLI:

```sh
pnpm install
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
```

Run the desktop app in development mode from the Studio directory:

```sh
cd studio
wails dev
```

Build a distributable app from the Studio directory:

```sh
wails build
```

Wails runs the Studio-mode Vite build and embeds its assets in the desktop app.
The static website build remains separate: `pnpm run build`.

The web preview displays sample data, with file and mutation actions disabled.
Launch the Wails app to use real local Projects. Studio dialogs support
keyboard navigation: Tab stays within an open dialog, Escape closes it, and
focus returns to the control that opened it.

## Microsoft Store

Studio uses the Go engine directly, so the engine is compiled into the app
executable; users do not need a separate engine installation. Wails builds
the Windows executable, and the Windows SDK packages it as MSIX. See the
[Microsoft Store packaging guide](store/README.md) for Partner Center identity
requirements and packaging instructions.
