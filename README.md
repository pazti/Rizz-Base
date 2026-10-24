# Rizzbase

Rizzbase is a local-first database for Node.js. Each Project is one encrypted,
Brainrot-encoded `.rizz` file on your computer. There is no cloud service,
network database, account, or realtime connection.

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

V1 includes the Go engine and CLI, TypeScript/Node.js and Python SDKs,
Project/Lobby/Yap operations, in-memory filters and ordering, and JSON
export/restore. The React product UI is a preview and is not connected to
local database files.

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
each SDK: Node.js uses `binaryPath`, Python uses `binary_path`, then both check
`RIZZBASE_BINARY` and `PATH`.

### Supported languages

- **Node.js / TypeScript** — Node.js 20+, install with `npm install rizzbase`.
- **Python** — Python 3.10+, install with `python -m pip install rizzbase`.

The SDKs never download the engine automatically; provide it using the
constructor option, `RIZZBASE_BINARY`, or `PATH`. See the [Node.js guide](docs/node-sdk.md),
[Python SDK guide](python/README.md), [installers and conformance details](docs/installation.md),
[conformance fixtures](conformance/README.md), and [runnable examples](examples/README.md).

> **What's new since 1.1.0:** checksum-verifying engine installers, a Python
> SDK, and safer engine discovery and compatibility checks. The SDKs still
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
passphrase. See the [SDK backup API and safety details](docs/node-sdk.md#project).

## Security and limits

- Choose a strong, unique passphrase and obtain it through private user input or
  a secrets manager. Do not commit, log, print, or place it in command-line
  arguments. The SDK sends it only to its local engine over stdin.
- Projects use AES-256-GCM with PBKDF2-HMAC-SHA256 at 600,000 iterations.
  Wrong passphrases and damaged ciphertext fail authentication. There is no
  passphrase recovery or key rotation.
- The SDK only runs an engine you provide through `binaryPath`,
  `RIZZBASE_BINARY`, or `PATH`, in that order. It checks the engine's reported
  version and project format before using it.
- Keep secure backups of `.rizz` files. JSON export is also available, but it
  contains decrypted data and must be protected accordingly. A local file lock
  blocks a second writer; after a crash, check that no process is still writing
  before manually removing its adjacent `.lock` directory.
- Each operation's JSON request is limited to 64 KiB; query results are
  bounded by the 64 MiB encoded-project file limit.
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
OS/architecture variants when a matching version tag is pushed; the current
source version expects `v1.1.0`.

See [SDK guide](docs/node-sdk.md), [file format and Go API](doc/FORMAT-legacy.md),
[contributing](CONTRIBUTING.md) and [security](SECURITY.md). Rizzbase is
licensed under the [MIT License](LICENSE).
