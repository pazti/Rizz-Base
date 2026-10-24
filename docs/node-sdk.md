# Node.js SDK

The Node.js SDK wraps the engine binary: each method resolves a user-provided binary, spawns one engine process with your request on stdin, and resolves with the decoded result. A Python SDK is also available; see the [SDK overview](./sdks.md).

Requires **Node.js 20+** and an installed Rizzbase engine. The SDK supports engine versions `>=1.1.0 <2.0.0` and project format `1`. Engine, SDK, and project-format versions are independent.

## Install

```sh
npm install rizzbase
```

Install the engine yourself (see the [Install page](./installation.md)). The SDK never downloads, caches, or executes a remote binary. It checks a resolved engine's version and project format once per binary path before its first request.

## Quickstart

```ts
import { Rizzbase } from "rizzbase";

const db = new Rizzbase("./my-project.rizz", getPassphrasePrivately());

// First run only — refuses to overwrite:
await db.create("My project");

// Later runs, open instead:
// const project = await db.open();

const lobby = await db.createLobby("general");
await db.createYap(lobby.id, "welcome", { message: "hello" });

const welcome = await db.getYap<{ message: string }>(lobby.id, "welcome");
const yaps = await db.query(lobby.id)
  .where("message", "==", "hello")
  .limit(10)
  .exec();

console.log(welcome.message, yaps.length);
```

`getPassphrasePrivately()` is your code — a prompt or secrets manager. Never hardcode, log, or commit it.

Constructor:

```ts
new Rizzbase(path, passphrase, options?)
```

| Option | Default | Meaning |
| --- | --- | --- |
| `binaryPath` | unset | Explicit trusted executable; takes precedence |
| `timeoutMs` | `30000` | Per-engine-call timeout |
| `retry` | safe defaults | Retry and lock-wait policy described below |

Binary resolution order is `binaryPath` → `RIZZBASE_BINARY` → `rizzbase` on `PATH`. If no executable is found, the SDK reports `BINARY_UNAVAILABLE` with searched locations and installation guidance. An incompatible or unverifiable engine reports `ENGINE_INCOMPATIBLE`.

Relative paths resolve from `process.cwd()`. Operations on one instance are serialized in call order; a failed write does not poison the queue. Cross-instance and cross-process behavior is defined in the [concurrency contract](./concurrency.md).

The retry policy accepts `maxAttempts` (default `3`, including the first attempt),
`baseDelayMs` (75), `maxDelayMs` (1000), `totalBudgetMs` (2000 and always less
than `timeoutMs`), and `lockWaitMs` (1000 and less than `timeoutMs`). Backoff
uses full jitter; the retry budget starts after the first timeout and includes
subsequent attempts plus their delays, bounding how long queued work can wait.
Only read-only operations (`open`, list, get, query) retry
`ENGINE_TIMEOUT`; authentication, validation, not-found, permission, and other
engine errors are returned immediately. Writes are never replayed after timeout
or `PROJECT_BUSY`; instead, the engine waits for an active lock up to
`lockWaitMs` and reports stale locks immediately.
Create/import are also not replayed because their outcome can be ambiguous.

```ts
const db = new Rizzbase("./my-project.rizz", getPassphrasePrivately(), {
  timeoutMs: 30_000,
  retry: { maxAttempts: 4, baseDelayMs: 75, maxDelayMs: 1000, totalBudgetMs: 2000, lockWaitMs: 1500 },
});
```

## API

All methods return Promises and reject with `RizzbaseError` (`code` + `message`, never includes the passphrase):

```ts
import { RizzbaseError } from "rizzbase";

try {
  await db.open();
} catch (error) {
  if (error instanceof RizzbaseError) console.error(error.code, error.message);
  throw error;
}
```

### Project

- `create(name): Promise<Project>` — new encrypted project; `FILE_EXISTS` if present.
- `open(): Promise<Project>` — decrypt + metadata.
- `doctor(): Promise<DoctorReport>` — non-destructive file, format, lock, permission, temporary-file, and engine diagnostics. The SDK supplies its passphrase for full decrypt and project validation; use the protocol directly for structural-only checks.
- `export(backupPath): Promise<void>` — write a decrypted JSON snapshot; never overwrites, owner-only permissions, 64 MiB cap. The backup is sensitive plaintext.
- `importBackup(backupPath): Promise<Project>` — validate a JSON backup and encrypt it into this instance's new `.rizz` path. Bad backups reject with `INVALID_BACKUP`.

Backup paths must end in `.json` and differ from the project path:

```ts
await db.export("./backup.json");

const restored = new Rizzbase("./restored.rizz", getPassphrasePrivately());
await restored.importBackup("./backup.json");
```

This export/import flow is also the supported way to rotate a passphrase:
export with the current passphrase, then import into a different `.rizz` path
using the new passphrase. It does not change a file in place. Stop writes and
verify the imported Project before switching applications. The JSON backup is
decrypted sensitive data; protect and remove it when no longer needed. See the
[security guide](./security.md#passphrase-rotation) for the full procedure.

For complete runnable apps, see the `examples/` folder in the repository (Notes and Todo).

### Lobby

- `createLobby(name): Promise<Lobby>` — generated `id` plus `name`, `createdAt`, `yaps`.
- `listLobbies(): Promise<Lobby[]>` — ID order.
- `deleteLobby(lobbyId): Promise<void>` — deletes Yaps too.

### Yap

- `createYap<T>(lobbyId, yapId, data): Promise<Yap<T>>`
- `createYaps<T>(lobbyId, yaps): Promise<Yap<T>[]>` — create 1–1000 Yaps atomically with one engine process and one project write. IDs must be unique in the batch and absent from the Lobby; conflicts reject the whole batch.
- `getYap<T>(lobbyId, yapId): Promise<T>` — returns `data` only.
- `listYaps<T>(lobbyId): Promise<Yap<T>[]>` — Yap-ID order.
- `updateYap<T>(lobbyId, yapId, data): Promise<Yap<T>>` — full replace.
- `deleteYap(lobbyId, yapId): Promise<void>`

`Yap<T>` is `{ id, data }`. Pass plain JSON only — no cycles, no class instances, no symbol keys. Numbers must be finite.

### Queries

Options form:

```ts
await db.queryYaps("LOBBY-ID", {
  where: [
    { field: "user.active", operator: "==", value: true },
    { field: "score", operator: ">=", value: 10 },
  ],
  orderBy: { field: "score", direction: "desc" },
  limit: 20,
});
```

For bounded cursor pagination, use `queryYapPage` (or the builder's
`execPage()`). It returns `{ yaps, nextCursor }`; pass the cursor with
`cursor`/`.after(cursor)` to fetch the next page. The cursor is opaque, tied to
the project, Lobby, and filter/order query, and independent of the page limit. An empty
`nextCursor` means there are no more matching results. Changing project data
between page requests is not snapshot-isolated and can change subsequent
results; restart pagination after writes.

```ts
const first = await db.query("LOBBY-ID").orderBy("score", "desc").limit(100).execPage();
if (first.nextCursor) {
  const second = await db.query("LOBBY-ID").orderBy("score", "desc")
    .limit(100).after(first.nextCursor).execPage();
}
```

Builder form (immutable — each `where`/`orderBy`/`limit` returns a new query):

```ts
await db.query("LOBBY-ID")
  .where("user.active", "==", true)
  .where("score", ">=", 10)
  .orderBy("score", "desc")
  .limit(20)
  .exec();
```

`orderBy(field, direction)` takes `"asc"` (default) or `"desc"`. Numbers sort before strings; missing or non-number/string values sort last in either direction; ties fall back to Yap ID. Same semantics as the CLI: AND-combined, dot paths, six operators, missing field never matches, filter → order → cursor → limit. Existing `queryYaps` and builder `exec()` still return arrays for compatibility; use page APIs for continuation.

### Error codes

`INVALID_ARGUMENT`, `BINARY_UNAVAILABLE`, `ENGINE_INCOMPATIBLE`, `ENGINE_TIMEOUT`, `ENGINE_PROTOCOL_ERROR`, `ENGINE_ERROR`, plus engine codes `AUTHENTICATION_FAILED`, `INVALID_FORMAT`, `PROJECT_BUSY`, `LOBBY_NOT_FOUND`, `YAP_NOT_FOUND`, `YAP_EXISTS`, `FILE_NOT_FOUND`, `FILE_EXISTS`, `PERMISSION_DENIED`, `INVALID_BACKUP`, and SDK-side `EXPORT_FAILED`.

`PROJECT_BUSY` includes lock owner details when metadata is readable. Use the
engine's `doctor` operation to distinguish active, stale, and unknown locks.
It never deletes a lock; do not remove an unknown lock or one owned by a live
process. The typed `RizzbaseError.context` carries the lock state, metadata,
and owner age when the engine can provide them.

## Typing

```ts
import type { Project, Lobby, Yap, Filter, OrderBy, OrderDirection, QueryOptions, JsonValue } from "rizzbase";
```

`JsonValue` is the standard JSON union. Generics like `getYap<{ message: string }>` only affect TypeScript — the engine stores raw JSON.
