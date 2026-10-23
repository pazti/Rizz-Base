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
- Passphrase goes in the JSON body on stdin. Never as an argument.

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
| `passphrase` | all | Nonempty, private stdin only |
| `name` | `create`, `lobby.create` | Project or Lobby display name |
| `backupPath` | `project.import` | Validated `.json` backup; must differ from `path` |
| `lobbyId` | `lobby.delete`, all `yap.*` | Generated Lobby ID |
| `yapId` | `yap.create/get/update/delete` | 1–128 bytes, caller-chosen |
| `data` | `yap.create`, `yap.update` | Valid JSON value |
| `query` | `yap.query` | `{ "where": [...], "orderBy": {...}, "limit": n }` |

Unknown fields are rejected.

## Operations

| Operation | Needs | Returns |
| --- | --- | --- |
| `create` | `name` | `{ ok:true, project }` — never overwrites an existing file |
| `open` | — | `{ ok:true, project }` — decrypt + metadata |
| `project.import` | `backupPath` | `{ ok:true, project }` — validate a `.json` backup and encrypt it into a new file; never overwrites |
| `lobby.create` | `name` | `{ ok:true, lobby }` with generated `id` |
| `lobby.list` | — | `{ ok:true, lobbies }` sorted by ID |
| `lobby.delete` | `lobbyId` | `{ ok:true }` — deletes contained Yaps too |
| `yap.create` | `lobbyId`, `yapId`, `data` | `{ ok:true, yap:{id,data} }` — duplicate is an error |
| `yap.get` | `lobbyId`, `yapId` | `{ ok:true, data }` |
| `yap.list` | `lobbyId` | `{ ok:true, yaps }` sorted by Yap ID |
| `yap.update` | `lobbyId`, `yapId`, `data` | `{ ok:true, yap }` — full replace |
| `yap.delete` | `lobbyId`, `yapId` | `{ ok:true }` |
| `yap.query` | `lobbyId`, `query` | `{ ok:true, yaps }` filtered, ordered, then limited |

Failure shape:

```json
{ "ok": false, "error": "human message", "errorCode": "YAP_NOT_FOUND" }
```

Error codes: `AUTHENTICATION_FAILED`, `INVALID_FORMAT`, `PROJECT_BUSY`, `LOBBY_NOT_FOUND`, `YAP_NOT_FOUND`, `YAP_EXISTS`, `FILE_NOT_FOUND`, `FILE_EXISTS`, `PERMISSION_DENIED`, `INVALID_BACKUP`, `ENGINE_ERROR`.

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

## Backup and restore

There is no `export` operation: exporting is decrypt + write JSON, which SDKs
do client-side (`db.export()` writes pretty-printed JSON with owner-only
permissions, capped at 64 MiB, and refuses to overwrite). Restoring is
`project.import`, which validates the complete decrypted Project structure
(wrong shape is `INVALID_BACKUP`) before creating the encrypted destination.
Both sides refuse to overwrite existing files, and the backup path must end
in `.json` while the destination ends in `.rizz`. Treat backups as sensitive
plaintext.

## Limits to design around

- Request body: 64 KiB. Large single `create`/`update` payloads are rejected.
- Encoded project file: 64 MiB.
- IDs: 1–128 bytes.
- Writes serialize on an adjacent `.lock` directory. A present lock blocks writes with `PROJECT_BUSY`. After a crash, verify no writer is alive before removing it.
- Writes are atomic rename after full sync; the old file stays intact until publish.
