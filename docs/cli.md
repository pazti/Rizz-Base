# CLI + engine reference

The `rizzbase` executable is the whole database. SDKs are thin wrappers around it. Learn this protocol and you can drive Rizzbase from any language.

## Calling convention

- Engine requests use no subcommands or flags. Usage is:
```text
rizzbase < request.json
```
- `rizzbase --version` writes one JSON object and exits 0 without reading stdin:
  `{"version":"1.1.0","projectFormatVersion":1}`. `version` is injected at
  build time and defaults to `dev`; `projectFormatVersion` describes the
  on-disk project format, independently of the engine version.
- `rizzbase --help` prints usage. Other extra argv is an error (exit 2).
- Reads **one** JSON object from stdin (max 64 KiB), writes **one** JSON object to stdout.
- One operation per process. Exit 0 on success, 1 on failure.
- Passphrase goes in the JSON body on stdin for project operations. It is optional for structural-only `doctor`; never pass it as an argument.

## Request shape

```json
{
  "operation": "yap.create",
  "path": "./my-project.rizz",
  "passphrase": "obtain-privately",
  "lobbyId": "LOBBY-ID",
  "yapId": "welcome",
  "data": { "message": "hello" }
}
```

| Field | Required for | Notes |
| --- | --- | --- |
| `operation` | all | See table below |
| `path` | all | Must end in `.rizz` (SDK rule; engine accepts any path) |
| `passphrase` | all except `doctor` | Private stdin only; omit on `doctor` for structural checks only |
| `name` | `create`, `lobby.create` | Project or Lobby display name |
| `backupPath` | `project.import` | Validated `.json` backup; must differ from `path` |
| `lobbyId` | `lobby.delete`, all `yap.*` | Generated Lobby ID |
| `yapId` | `yap.create/get/update/delete` | 1–128 bytes, caller-chosen |
| `data` | `yap.create`, `yap.update` | Valid JSON value |
| `query` | `yap.query` | `{ "where": [...], "orderBy": {...}, "limit": n }` |
| `lockWaitMs` | write operations | Optional bounded wait for the adjacent project lock; 0–60000 ms |

Unknown fields are rejected.

## Operations

| Operation | Needs | Returns |
| --- | --- | --- |
| `doctor` | `path` | `{ ok:true, doctor }` — non-destructive file, format, lock, permission, temp-file, and engine diagnostics; include `passphrase` for full decrypt + project validation |
| `create` | `name` | `{ ok:true, project }` — never overwrites an existing file |
| `open` | — | `{ ok:true, project }` — decrypt + metadata |
| `project.import` | `backupPath` | `{ ok:true, project }` — validate a `.json` backup and encrypt it into a new file; never overwrites |
| `lobby.create` | `name` | `{ ok:true, lobby }` with generated `id` |
| `lobby.list` | — | `{ ok:true, lobbies }` sorted by ID |
| `lobby.delete` | `lobbyId` | `{ ok:true }` — deletes contained Yaps too |
| `yap.create` | `lobbyId`, `yapId`, `data` | `{ ok:true, yap:{id,data} }` — duplicate is an error |
| `yap.bulkCreate` | `lobbyId`, `yaps` | `{ ok:true, yaps }` — atomically creates 1–1000 items; any duplicate rejects the entire batch |
| `yap.get` | `lobbyId`, `yapId` | `{ ok:true, data }` |
| `yap.list` | `lobbyId` | `{ ok:true, yaps }` sorted by Yap ID |
| `yap.update` | `lobbyId`, `yapId`, `data` | `{ ok:true, yap }` — full replace |
| `yap.delete` | `lobbyId`, `yapId` | `{ ok:true }` |
| `yap.query` | `lobbyId`, `query` | `{ ok:true, yaps, page:{ yaps, nextCursor } }` filtered, ordered, cursor-resumed, then limited |

Failure shape:

```json
{ "ok": false, "error": "human message", "errorCode": "YAP_NOT_FOUND" }
```

`PROJECT_BUSY` may also include an `errorContext` object with lock state,
owner metadata, and age. Error codes: `AUTHENTICATION_FAILED`, `INVALID_FORMAT`,
`PROJECT_BUSY`, `LOBBY_NOT_FOUND`, `YAP_NOT_FOUND`, `YAP_EXISTS`,
`FILE_NOT_FOUND`, `FILE_EXISTS`, `PERMISSION_DENIED`, `INVALID_BACKUP`,
`ENGINE_ERROR`.

## Examples

Read the passphrase without echoing it, then construct each request from stdin.
The passphrase is not stored in shell history or passed as a process argument:

```sh
read -r -s -p "Project passphrase: " RIZZBASE_PASSPHRASE
printf '\n'
printf '%s' "$RIZZBASE_PASSPHRASE" | jq -Rs '{operation:"create",path:"./demo.rizz",passphrase:.,name:"My project"}' | rizzbase
printf '%s' "$RIZZBASE_PASSPHRASE" | jq -Rs '{operation:"lobby.create",path:"./demo.rizz",passphrase:.,name:"general"}' | rizzbase
unset RIZZBASE_PASSPHRASE
```

These Bash/Zsh commands require `jq`. In other environments, use a private
prompt or secret manager and pass the request to `rizzbase` through stdin.

Query with two filters, an ordering, and a limit:

```json
{
  "operation": "yap.query",
  "path": "./demo.rizz",
  "passphrase": "<provide privately>",
  "lobbyId": "LOBBY-ID",
  "query": {
    "where": [
      { "field": "user.active", "operator": "==", "value": true },
      { "field": "score", "operator": ">=", "value": 10 }
    ],
    "orderBy": { "field": "score", "direction": "desc" },
    "limit": 20
  }
}
```

Operators: `==`, `!=`, `>`, `>=`, `<`, `<=`. Filters are AND-combined. `field` uses dot-separated paths (`user.active`). Equality compares full JSON values (objects and arrays included). Ordering only compares number-vs-number or string-vs-string. Missing fields never match, including `!=`. `orderBy` direction is `"asc"` (default) or `"desc"`; numbers sort before strings, missing or non-number/string values sort last in either direction, and ties fall back to Yap ID. Filtering, ordering, then limiting happen in that order. The Lobby is scanned in memory; no indexes.

Use `query.cursor` from the previous page's `page.nextCursor` to continue.
Cursors are opaque, bound to the project, Lobby, and filter/order query, and
reusable with a different page limit. A cursor is invalid if it is malformed, belongs to
another query, or its last Yap no longer matches. Pagination is not a
snapshot; writes between page requests may change later pages. Restart after
mutations.

## Backup and restore

There is no `export` operation: exporting is decrypt + write JSON, which SDKs
do client-side (`db.export()` writes pretty-printed JSON with owner-only
permissions, capped at 64 MiB, and refuses to overwrite). Restoring is
`project.import`, which validates the complete decrypted Project structure
(wrong shape is `INVALID_BACKUP`) before creating the encrypted destination.
Both sides refuse to overwrite existing files, and the backup path must end
in `.json` while the destination ends in `.rizz`. Treat backups as sensitive
plaintext. For passphrase rotation, export with the current passphrase and
import into a different `.rizz` destination using the new passphrase. This
decrypts and re-encrypts the whole Project; it does not modify the original
file in place. Stop writes during the migration and verify the new file before
switching applications. See the [security guide](./security.md#passphrase-rotation)
for the recommended procedure.

## Limits to design around

- Request body: 64 KiB. Large single `create`/`update` payloads are rejected.
- Encoded project file: 64 MiB.
- IDs: 1–128 bytes.
- Writes serialize on an adjacent `.lock` directory containing `lock.json` owner metadata. Without `lockWaitMs`, a held lock immediately returns `PROJECT_BUSY`; with it, the engine waits only up to that bound for an active lock. Stale locks return immediately. The error identifies the PID/host when available.
- `doctor` reports whether a lock owner is active, stale, or unknown. It never removes a lock; malformed metadata and locks on another host are unknown, not stale. Only consider manual removal after checking the owner and confirming no writer can still run.
- Writes are atomic rename after full sync; the old file stays intact until publish.
- The supported engine/project-format matrix and local-only locking scope are documented in the [SDK overview](./sdks.md) and [concurrency contract](./concurrency.md).
