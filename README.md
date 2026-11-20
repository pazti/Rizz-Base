<p align="center">
  <a href="https://github.com/pazti/Rizz-Base">
    <img src="public/brand/rizzbase-wordmark-on-dark.svg" alt="Rizzbase" width="270" />
  </a>
</p>

<p align="center">
  <strong>Local-first storage. Your data stays on your machine.</strong><br />
  Encrypted Projects in one portable <code>.rizz</code> file. No hosted database, accounts, or realtime service.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/LOCAL--FIRST-PRIVATE%20BY%20DESIGN-0B0F0E?style=for-the-badge&labelColor=0B0F0E&color=38BFA2" alt="Local-first, private by design" />
  <img src="https://img.shields.io/badge/ENCRYPTION-AES--256--GCM-0B0F0E?style=for-the-badge&labelColor=0B0F0E&color=38BFA2" alt="AES-256-GCM encryption" />
  <a href="https://github.com/pazti/Rizz-Base/releases/tag/studio-v1.0.0">
    <img src="https://img.shields.io/badge/STUDIO-WINDOWS%20APP-0B0F0E?style=for-the-badge&labelColor=0B0F0E&color=38BFA2" alt="Download Rizzbase Studio for Windows" />
  </a>
</p>

<p align="center">
  <a href="#technology-stack"><strong>Technology stack</strong></a> ·
  <a href="#install"><strong>Install</strong></a> ·
  <a href="#rizzbase-studio"><strong>Rizzbase Studio</strong></a> ·
  <a href="docs/node-sdk.md"><strong>Documentation</strong></a>
</p>

## Why Rizzbase?

- **Your data stays local.** No hosted database, account, or network service.
- **Encrypted Projects.** AES-256-GCM protects one portable `.rizz` file with
  a key derived from your passphrase.
- **Ordinary TypeScript.** Store JSON documents and query them with a small,
  typed API backed by a Go engine.
- **Back up on your terms.** Keep encrypted `.rizz` copies, or export a
  decrypted JSON snapshot when you need an interoperable backup.
- **Small, transparent scope.** Rizzbase is a local file library, not a
  realtime cloud platform. Queries scan a Lobby in memory; there are no indexes.

V1 includes the Go engine and CLI, official TypeScript/Node.js, Python, and Go
SDKs, Project/Lobby/Yap operations, append-only Events, in-memory filters and
ordering, and JSON export/restore. See [Events](docs/events.md) for SDK usage.
The React product UI is a preview and is not connected to local database files.

## Technology stack

<p>
  <img src="https://img.shields.io/badge/Go-engine-0B0F0E?style=for-the-badge&logo=go&logoColor=00ADD8" alt="Go engine" />
  <img src="https://img.shields.io/badge/Go-Official%20SDK-0B0F0E?style=for-the-badge&logo=go&logoColor=00ADD8" alt="Official Go SDK" />
  <img src="https://img.shields.io/badge/Node.js-SDK-0B0F0E?style=for-the-badge&logo=nodedotjs&logoColor=5FA04E" alt="Node.js SDK" />
  <img src="https://img.shields.io/badge/TypeScript-typed%20API-0B0F0E?style=for-the-badge&logo=typescript&logoColor=3178C6" alt="TypeScript SDK" />
  <img src="https://img.shields.io/badge/Python-SDK-0B0F0E?style=for-the-badge&logo=python&logoColor=FFD43B" alt="Python SDK" />
  <img src="https://img.shields.io/badge/Wails-desktop-0B0F0E?style=for-the-badge&logo=wails&logoColor=38BFA2" alt="Wails desktop app" />
  <img src="https://img.shields.io/badge/React-UI-0B0F0E?style=for-the-badge&logo=react&logoColor=61DAFB" alt="React UI" />
  <img src="https://img.shields.io/badge/Vite-build-0B0F0E?style=for-the-badge&logo=vite&logoColor=646CFF" alt="Vite build tooling" />
  <img src="https://img.shields.io/badge/Tailwind%20CSS-styles-0B0F0E?style=for-the-badge&logo=tailwindcss&logoColor=06B6D4" alt="Tailwind CSS" />
</p>

- **Core:** Go engine and CLI; encrypted Projects use AES-256-GCM.
- **SDKs:** Node.js with TypeScript, Python, and the official Go process client.
- **Desktop:** Wails app with a React interface.
- **Web preview:** React, Vite, and Tailwind CSS; it uses sample data and
  cannot access local `.rizz` files.

## Install

> **Important: the SDKs never download the engine.** Install the engine
> separately first, then install the language SDK you want to use.

### 1. Install the engine

On Linux or macOS, download and inspect the
[installer script](https://github.com/pazti/Rizz-Base/blob/main/install.sh),
then run it:

```sh
curl -fLO https://raw.githubusercontent.com/pazti/Rizz-Base/main/install.sh
less install.sh
chmod +x install.sh
./install.sh
```

On Windows, inspect the [PowerShell installer](https://github.com/pazti/Rizz-Base/blob/main/install.ps1)
before running it:

```powershell
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/pazti/Rizz-Base/main/install.ps1" -OutFile ".\install.ps1"
Get-Content .\install.ps1
.\install.ps1
```

The installers verify release-binary checksums and install the engine under
`~/.rizzbase/bin` (Windows: `%USERPROFILE%\.rizzbase\bin`). Add that directory
to `PATH` as prompted. Alternatively, download a release binary and verify it
using the instructions in [Engine installation](docs/installation.md), or
[build the engine from source](docs/installation.md#manual-installation).

### 2. Install an SDK

For Go, use the official engine-native API:

```sh
go get rizzbase/engine/core
```
See the [Go SDK guide](docs/go-sdk.md).

For Node.js:

```sh
npm install rizzbase
```

The Node SDK requires Node.js 20 or newer and the separately installed engine.
Python is optional; its SDK requires Python 3.10 or newer. Each SDK checks
engine and project-format compatibility before its first request.

For Python, install Python 3.10 or newer, then install the SDK from PyPI:

```sh
python -m pip install rizzbase
```

See [engine installation](docs/installation.md) for version pinning, manual
binary downloads, and platform details. Binary selection is configurable in
each SDK: Node.js uses `binaryPath`, Python uses `binary_path`, and Go uses
`WithBinaryPath`; all fall back to `RIZZBASE_BINARY` and `PATH`.

### Supported languages

- **Go** — official engine-native API; import `rizzbase/engine/core`.
- **Node.js / TypeScript** — Node.js 20+, install with `npm install rizzbase`.
- **Python** — Python 3.10+, install with `python -m pip install rizzbase`.
- **Go** — Go 1.24+, install with `go get github.com/pazti/Rizz-Base/sdk/go`.

[SDKs](docs/sdks.md) never download the engine automatically; provide it using
the constructor option, `RIZZBASE_BINARY`, or `PATH`. See the [Node.js guide](docs/node-sdk.md),
[Python SDK guide](python/README.md), [Go SDK guide](sdk/go/README.md), [installers and conformance details](docs/installation.md),
[conformance fixtures](conformance/README.md), and [runnable examples](examples/README.md).

> **What's new since 1.1.0:** checksum-verifying engine installers, Python and
> Go SDKs, and safer engine discovery and compatibility checks. The SDKs still
> require an engine you install or build yourself.

## 60-second quick start

Requires Node.js 20+, the separately installed engine, and a supported macOS,
Linux, or Windows x64/arm64 machine. Install the SDK:

```sh
npm install rizzbase
read -rsp "Project passphrase: " RIZZBASE_PASSPHRASE
printf '\n'
export RIZZBASE_PASSPHRASE
```

These passphrase commands are for Bash/Zsh. In other environments, use a
private prompt or secret manager. Do not commit the passphrase or put it in
application arguments. Save this as `quick-start.mjs`:

```js
import { Rizzbase, RizzbaseError } from "rizzbase";

const passphrase = process.env.RIZZBASE_PASSPHRASE;
if (!passphrase) throw new Error("Set RIZZBASE_PASSPHRASE privately.");

const db = new Rizzbase("./journal.rizz", passphrase);
try {
  await db.open();
} catch (error) {
  if (!(error instanceof RizzbaseError) || error.code !== "FILE_NOT_FOUND") throw error;
  await db.create("Personal journal");
}
const notes = (await db.listLobbies()).find(lobby => lobby.name === "notes")
  ?? await db.createLobby("notes");

await db.createYap(notes.id, `note-${Date.now()}`, {
  title: "Hello, local storage",
  text: "This is encrypted in my local Project file.",
  createdAt: new Date().toISOString(),
});

const latest = await db.query(notes.id)
  .limit(10)
  .exec();
console.log(latest);
```

Run `node quick-start.mjs`. The example opens an existing Project if present;
`create()` itself intentionally refuses to overwrite one.

## Examples

The repository includes interactive [Notes](examples/notes.mjs) and
[Todo list](examples/todos.mjs) apps. They demonstrate persistent data,
filtered/ordered queries, and updates. See [how to run the examples](examples/README.md).
The example scripts import the SDK from this repository, so build the local
SDK and engine before running them.

## Build and test from source

```sh
cd sdk
npm install
npm run build
npm test
```

## Deploy the UI to Vercel

The React preview is a Vite static site. Import this repository into Vercel
with the repository root as the project root; Vercel detects Vite
automatically. Use `npm run build` as the build command and `dist` as the output
directory if setting these manually. The app uses hash-based navigation, so it
does not require server rewrites. The deployed UI remains a preview using sample data. It is not connected to
local `.rizz` files and does not provide hosted database features.

## Rizzbase Studio

Rizzbase Studio is the Wails desktop app for creating and opening local
Projects, browsing and editing Lobbies and Yaps, and exporting JSON backups.
It calls the Go engine directly and does not add a hosted database or account
service. See the [Studio development guide](studio/README.md) for prerequisites
and build instructions. The Go engine is compiled into the Studio executable;
users do not need to install an external engine binary to use Studio. The
browser preview remains separate and cannot access local `.rizz` files.

The [Microsoft Store packaging guide](studio/store/README.md) covers MSIX
generation and Partner Center setup. The public
[privacy policy](https://rizzbase.tech/privacy/) describes data handling
for Studio, the SDKs, command-line tools, and website. For general or privacy
questions, email [hello@rizzbase.tech](mailto:hello@rizzbase.tech); for direct
contact, reach [Paul](mailto:paul@rizzbase.tech).

## Reopen an existing Project

```ts
import { Rizzbase } from "rizzbase";

const passphrase = process.env.RIZZBASE_PASSPHRASE;
if (!passphrase) throw new Error("Set RIZZBASE_PASSPHRASE privately.");
const db = new Rizzbase("./journal.rizz", passphrase);
const project = await db.open();
const [notes] = await db.listLobbies();
const recent = await db.query(notes.id).limit(10).exec();
console.log(project.name, recent.length);
```

`create()` is for first-time project setup and refuses to overwrite an existing
file. Keep the project path and passphrase private. Read the
[beginner-friendly guide and API reference](docs/node-sdk.md).

## Project vocabulary

- **Project**: one encrypted `.rizz` file. It holds its name, metadata and Lobbies.
- **Lobby**: a named group in a Project, with a generated ID and a set of Yaps.
- **Yap**: a JSON document with an ID you choose, stored inside a Lobby.

## Query example

Filters are AND-combined, dot-separated paths access nested object fields, and
the optional limit is applied after filtering and ordering. Without `orderBy`,
results are sorted by Yap ID. Queries read the Lobby into memory; V1 has no
indexes or persisted query engine.

```ts
const active = await db.query("YOUR-LOBBY-ID")
  .where("user.active", "==", true)
  .where("score", ">=", 10)
  .orderBy("score", "desc")
  .limit(20)
  .exec();
```

Supported operators: `==`, `!=`, `>`, `>=`, `<`, `<=`. Equality compares JSON
values (including objects and arrays); filter comparisons support numbers and
strings. `orderBy` sorts by a numeric or string field before `limit`; see
[query details](docs/node-sdk.md#queries).

## JSON backup and restore

```ts
await db.export("./backup.json");

const restored = new Rizzbase("./restored.rizz", getPassphrasePrivately());
await restored.importBackup("./backup.json");
```

JSON backups contain decrypted project data. Store them securely, do not commit
them, and remove them when no longer needed. Export and import refuse to
overwrite files; restore creates an encrypted Project using the supplied
passphrase. Export/import into a new `.rizz` path is also the supported way to
rotate a passphrase; it is not an in-place key change. Stop writes and verify
the imported project before switching applications. See the
[SDK backup API](docs/node-sdk.md#project) and
[rotation procedure](docs/security.md#passphrase-rotation).

## Security and limits

- Choose a strong, unique passphrase and obtain it through private user input or
  a secrets manager. Do not commit, log, print, or place it in command-line
  arguments. The SDK sends it only to its local engine over stdin.
- Projects use AES-256-GCM with PBKDF2-HMAC-SHA256 at 600,000 iterations.
  Wrong passphrases and damaged ciphertext fail authentication. There is no
  passphrase recovery; rotation requires the current passphrase and a protected
  plaintext JSON export/import migration.
- The SDK only runs an engine you provide through `binaryPath`,
  `RIZZBASE_BINARY`, or `PATH`, in that order. It checks the engine's reported
  version and project format before using it.
- Keep secure backups of `.rizz` files. JSON export is also available, but it
  contains decrypted data and must be protected accordingly. A local file lock
  blocks a second writer; after a crash, check that no process is still writing
  before manually removing its adjacent `.lock` directory.
- Each operation's JSON request is limited to 64 KiB; query results are
  bounded by the 64 MiB encoded-project file limit.
- The proposal for a larger V1+ Project limit remains tracked in
  [Project size scaling proposal](docs/project-size-scaling.md). The 64 MiB
  limit remains unchanged unless an approved implementation increases it.
- Performance depends on encrypted Project size; see the
  [measured engine benchmarks](docs/benchmarks.md) before choosing a Lobby
  capacity.
- This is local-only V1: no realtime, cloud storage, multi-user access, remote
  backup, or UI connection. The Go API and CLI have their own documented limits.

## Develop this repository

```sh
cd engine
go test -race ./...
go vet ./...
cd ../sdk
npm install
npm test
npx tsc --noEmit --strict --target ES2022 --module Node16 \
  --moduleResolution Node16 --types node test/typecheck.ts
```

Go 1.24+ and Node.js 20+ are required. The SDK's tests build the local Go CLI
and exercise it using temporary encrypted projects. Binary releases build six
OS/architecture variants when a matching version tag is pushed. The latest
engine release is `v1.3.0`; the official Go SDK follows the engine module,
while Node.js and Python SDK packages remain on their independent `1.1.0`
version line.

See [SDK guide](docs/node-sdk.md), [file format and Go API](doc/FORMAT-legacy.md),
[contributing](CONTRIBUTING.md) and [security](SECURITY.md). Rizzbase is
licensed under the [MIT License](LICENSE).
