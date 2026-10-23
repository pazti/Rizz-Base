# SDKs in other languages

**Node.js and Python** are production-ready. Each SDK owns only its language
API and binary resolution; both use the same engine protocol of one JSON
request on stdin and one JSON response on stdout. The CLI reference is the
behavioral specification.

## Status

| Language | Status | Notes |
| --- | --- | --- |
| Node.js / TypeScript | Stable (V1) | `npm install rizzbase`, see Node.js SDK page |
| Python | Stable (V1) | `python/`, mirrors the Node API; no auto-download |
| Go | Engine-native | Import `rizzbase/engine/core` directly, no CLI hop needed |

The Python package is under [`python/`](../python/README.md). Both SDKs use
the same offline scenarios in [`conformance/`](../conformance/README.md).

## Python API

The Python API mirrors Node with Python's standard synchronous conventions:

```python
from rizzbase import Rizzbase

db = Rizzbase("./my-project.rizz", get_passphrase_privately())

# db.create("My project")   # first run only
# db.open()                  # later runs

lobby = db.create_lobby("general")
db.create_yap(lobby["id"], "welcome", {"message": "hello"})
welcome = db.get_yap(lobby["id"], "welcome")
yaps = db.query(lobby["id"]).where("message", "==", "hello").order_by("score", "desc").limit(10).execute()
db.export("./backup.json")
```

Python SDK behavior:

- Constructor takes `(path, passphrase, binary_path=None, timeout_s=30)`.
- Binary resolution order: explicit `binary_path` → `RIZZBASE_BINARY` env → `PATH` → error with install instructions. No silent download.
- Check `rizzbase --version` before the first request and reject unsupported engine or project-format versions with a typed compatibility error.
- Shared offline behavior fixtures live in [`conformance/`](../conformance/README.md); add a runner for each SDK so implementations use the same expected results.
- Errors raise `RizzbaseError` with stable `code` strings matching the CLI `errorCode` set.
- Queries validate locally (`field` dot-paths, six operators, non-negative integer limit) before spawning the engine.

## Build your own SDK in 20 minutes

1. Resolve the `rizzbase` executable (explicit path → env → `PATH`).
2. Serialize `{ operation, path, passphrase, ... }` to stdin (cap 64 KiB).
3. Spawn with no shell, no passphrase in argv/env, with a timeout.
4. Parse stdout JSON: `ok:true` → return payload key (`project/lobby/lobbies/yap/yaps/data`); `ok:false` → raise typed error from `errorCode`.
5. Validate JSON values client-side (plain data, finite numbers) so engine errors stay rare.

Any language that can spawn a process and speak JSON can be a first-class SDK.
