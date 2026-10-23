# Rizzbase SDK guide

The SDK is a local Node.js wrapper around Rizzbase's Go engine. It never starts
a network database. It invokes the engine once per operation and stores data
in the `.rizz` file at the path you provide.

This older guide is retained for reference. For the current API, engine
installation, binary resolution and compatibility policy, see the
[current Node SDK guide](../docs/node-sdk.md).

## Requirements and installation

- Node.js 20 or newer.
- An installed Rizzbase engine executable.
- The Node package: `npm install rizzbase`.

The SDK never downloads or caches an engine. It resolves the executable in
this order: `binaryPath`, `RIZZBASE_BINARY`, then `rizzbase` on `PATH`. It
checks engine and project-format compatibility before its first operation.

## Quick start

```ts
import { Rizzbase } from "rizzbase";

const db = new Rizzbase("./my-project.rizz", getPassphrasePrivately());

// First run only:
await db.create("My project");

// On a later run, use this instead:
// const project = await db.open();

const lobby = await db.createLobby("general");
await db.createYap(lobby.id, "welcome", { message: "hello" });
const welcome = await db.getYap<{ message: string }>(lobby.id, "welcome");

console.log(welcome.message);
```

`getPassphrasePrivately()` above stands for your own secure input flow; it is
not part of the SDK. `create()` refuses to replace an existing project. Keep
the same passphrase to open it later. You can use other SDK operations directly
after constructing an instance; they read the existing project.

The SDK constructor is `new Rizzbase(path, passphrase, options?)`. Relative
paths resolve from the current working directory.

For complete runnable Notes and Todo examples, see the repository
[examples folder](../examples/README.md).

### Options

```ts
new Rizzbase("./data.rizz", passphrase, {
  binaryPath: "/trusted/path/to/rizzbase", // optional; highest resolution priority
  timeoutMs: 30_000,                       // optional; per engine call
});
```

The SDK supports engine versions `>=1.1.0 <2.0.0` and project format `1`.

## API reference

All database methods return Promises. Failures reject with `RizzbaseError`,
which has a stable `code` and a human-readable `message`. It does not include
the passphrase. Handle it with:

```ts
import { RizzbaseError } from "rizzbase";

try {
  await db.open();
} catch (error) {
  if (error instanceof RizzbaseError) {
    console.error(error.code, error.message);
  }
  throw error;
}
```

### Project

- `new Rizzbase(path, passphrase, options?)`: create a local SDK instance.
- `create(name): Promise<Project>`: create a new encrypted project; never
  overwrites an existing file.
- `open(): Promise<Project>`: decrypt and return the Project metadata.
- `export(backupPath): Promise<void>`: write a decrypted JSON backup without
  overwriting an existing file.
- `importBackup(backupPath): Promise<Project>`: validate a JSON backup and
  encrypt it into this instance's new `.rizz` path.

Errors reject as `RizzbaseError` with a stable code, for example
`INVALID_ARGUMENT` for invalid paths or query options, `LOBBY_NOT_FOUND` and
`YAP_NOT_FOUND` for missing records, `BINARY_UNAVAILABLE` when no executable
can be resolved, or `ENGINE_INCOMPATIBLE` when the engine version or project
format is unsupported. Error messages do not include the passphrase.

### Lobby

- `createLobby(name): Promise<Lobby>`: add a Lobby; its `id` is generated.
- `listLobbies(): Promise<Lobby[]>`: list Lobbies in ID order.
- `deleteLobby(lobbyId): Promise<void>`: delete a Lobby and all its Yaps.

### Yap

- `createYap<T extends JsonValue>(lobbyId, yapId, data): Promise<Yap<T>>`:
  create a JSON document. IDs are caller-supplied, 1–128 bytes.
- `getYap<T = JsonValue>(lobbyId, yapId): Promise<T>`: return document data.
- `listYaps<T = JsonValue>(lobbyId): Promise<Yap<T>[]>`: list records in Yap-ID order.
- `updateYap<T extends JsonValue>(lobbyId, yapId, data): Promise<Yap<T>>`:
  replace the entire document.
- `deleteYap(lobbyId, yapId): Promise<void>`: delete a Yap.

`Yap<T>` is `{ id: string; data: T }`. `Lobby` includes `id`, `name`,
`createdAt` and `yaps`; `Project` includes `version`, `name`, `createdAt` and
`lobbies`. Pass plain JSON-compatible values (objects, arrays, strings,
numbers, booleans and `null`), not cyclic values or class instances.

### Queries

Run an options query:

```ts
const results = await db.queryYaps("LOBBY-ID", {
  where: [
    { field: "user.active", operator: "==", value: true },
    { field: "score", operator: ">=", value: 10 },
  ],
  orderBy: { field: "score", direction: "desc" },
  limit: 20,
});
```

Or use the immutable query builder:

```ts
const results = await db.query("LOBBY-ID")
  .where("user.active", "==", true)
  .where("score", ">=", 10)
  .orderBy("score", "desc")
  .limit(20)
  .exec();
```

`orderBy(field, direction)` accepts `"asc"` or `"desc"`; the direction defaults
to `"asc"`. Numeric and string field values sort within their respective types,
with numbers before strings. Missing fields and values that are not numbers or
strings sort last in either direction. Ties are sorted by Yap ID. Filters are
applied first, then ordering, then the optional non-negative safe-integer
limit. Queries decrypt and scan the Lobby in memory; there are no indexes,
joins, or query plans.

Operators are `==`, `!=`, `>`, `>=`, `<` and `<=`. Multiple filters are ANDed.
Use dot-separated paths for nested JSON object fields. All six comparisons
support JSON equality for primitives, arrays and objects. Filter comparisons
apply only to values where both sides are numbers or both are strings; they
never coerce types. Missing fields do not match any filter, including `!=`.

### JSON export and restore

`export()` writes a decrypted JSON snapshot of the whole Project. The backup
contains the project name, metadata, Lobbies, and all Yap data in plain text:

```ts
await db.export("./backup.json");

// Restore to a new encrypted Project using a new passphrase.
const restored = new Rizzbase("./restored.rizz", getPassphrasePrivately());
await restored.importBackup("./backup.json");
```

Exports and imports refuse to overwrite existing files. Exported files are
created with owner-only permissions where supported, and JSON backups are
limited to 64 MiB. Treat the backup as sensitive plaintext: store it securely,
do not commit it, and remove it when no longer needed. Import validates the
complete Project structure before creating the encrypted destination file.
The destination must be a `.rizz` path; the backup must be a `.json` path.

## Project / Lobby / Yap

Think of one `.rizz` Project as the database file. Lobbies group related records
inside it (for example `users` or `messages`). Each Yap is a JSON document with
a unique ID within its Lobby. Lobbies receive generated IDs; the app chooses
Yap IDs. Deleting a Lobby also deletes all Yaps inside it.

## Brainrot encoding and encryption

The encrypted binary file is encoded into pairs of words from a fixed,
versioned vocabulary. Encoding is only a reversible representation; it is not
encryption and does not make data private. AES-256-GCM protects the contents
and detects tampering. A random salt and nonce are used, and PBKDF2-HMAC-SHA256
derives the encryption key from the passphrase (600,000 iterations). File
format details are in [FORMAT.md](FORMAT.md).

## Passphrase and file safety

- Obtain passphrases from a private prompt or secure secret store. Never
  hardcode, commit, print or log them, and never put them in CLI arguments.
- The local SDK sends the passphrase over the child process's stdin. It is not
  put in the process arguments or environment. Do not expose the SDK through
  an untrusted server endpoint.
- There is no recovery if the passphrase is lost. Keep secure encrypted
  backups; JSON exports are decrypted and need extra protection.
- Projects are local files with a 64 MiB encoded-file limit and owner-only
  permissions on Unix. Project writes use an adjacent lock; concurrent writers
  fail rather than overwrite each other. Check for an active process before
  manually clearing a lock left after a crash.
- Each request sent to the engine is limited to 64 KiB, so a single
  create/update with a larger JSON value is rejected. Query responses are
  bounded separately by the engine's project file-size limit.
- Downloads use HTTPS and verify a SHA-256 checksum delivered with the release.
  The manifest is not a cryptographic signature or independent proof of who
  published the binary. For a stronger supply-chain policy, build and audit
  the Go binary yourself, then configure `binaryPath`.
- Do not put `.rizz` files, passphrases, or real private data in source control.

## V1 boundaries

Rizzbase is local-only. It does not provide a server, realtime or multi-user
features, account management, remote backups, schema enforcement, or indexes.
The React product UI is marked Coming Soon: its dashboard uses mock data and
is not wired to the local engine. For the Go API, CLI request protocol,
file-size limit and atomic write details, see [FORMAT.md](FORMAT.md).
