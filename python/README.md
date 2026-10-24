# Rizzbase Python SDK

Python 3.10+ SDK for the local Rizzbase engine. **Install the engine
separately first; the SDK never downloads or installs it automatically.**

## Install

Install the engine using the Linux/macOS or Windows PowerShell installer, or a
verified release binary, following the
[engine installation guide](https://github.com/pazti/Rizz-Base/blob/main/docs/installation.md).
Then install the Python package from PyPI:

```sh
python -m pip install rizzbase
```

The engine must be on `PATH` or configured through `RIZZBASE_BINARY`. The
constructor's `binary_path` option takes precedence over both.

## Quick start

```python
from getpass import getpass

from rizzbase import Rizzbase

db = Rizzbase("./my-project.rizz", getpass("Project passphrase: "))

# First run only; create() refuses to overwrite an existing Project.
db.create("My project")
# On later runs, use db.open() instead.

lobby = db.create_lobby("general")
db.create_yap(lobby["id"], "welcome", {"message": "hello"})
welcome = db.get_yap(lobby["id"], "welcome")
yaps = (
    db.query(lobby["id"])
    .where("message", "==", "hello")
    .limit(10)
    .execute()
)
```

For SDK binary selection and compatibility requirements, see the
[engine installation guide](https://github.com/pazti/Rizz-Base/blob/main/docs/installation.md)
and [Node.js SDK guide](https://github.com/pazti/Rizz-Base/blob/main/docs/node-sdk.md).

The synchronous API includes project creation/open, backup export/import,
lobby and Yap CRUD, and query filtering/ordering. Failures raise
`RizzbaseError` with a stable `code` and safe message. The SDK supports engine
versions `>=1.1.0 <2.0.0` and project format `1`; it checks compatibility once
per resolved binary path.

`Rizzbase(..., retry={...})` optionally configures `max_attempts` (default 3),
`base_delay_ms` (75), `max_delay_ms` (1000), `total_budget_ms` (2000), and
`lock_wait_ms` (1000); time budgets must be less than `timeout_s`. Only
read-only calls retry `ENGINE_TIMEOUT` with full-jitter exponential backoff.
The retry budget begins after the first timeout and includes subsequent calls
and delays.
Writes are never replayed after a timeout; they wait for an existing writer
only up to `lock_wait_ms`. `db.doctor()` returns non-destructive diagnostics
and performs full decrypt/validation using the instance passphrase.
`RizzbaseError.context` carries structured lock owner details for
`PROJECT_BUSY` when available.
Cross-instance and cross-process behavior is described in the
[concurrency contract](../docs/concurrency.md).

### Passphrase rotation

There is no passphrase recovery or in-place key change. If you know the
current passphrase, rotate by exporting a plaintext JSON backup and importing
it into a new encrypted Project with a new passphrase:

```python
from getpass import getpass
from rizzbase import Rizzbase

source = Rizzbase("./project.rizz", getpass("Current passphrase: "))
source.export("./project-rotation.json")

destination = Rizzbase("./project-rotated.rizz", getpass("New passphrase: "))
destination.import_backup("./project-rotation.json")
destination.open()  # Verify the new Project before switching applications.
```

Stop writes during migration, protect the backup because it contains
decrypted Project data, and keep the source until the imported Project has
been verified. Export and import do not overwrite existing files. See the
[security guide](../docs/security.md#passphrase-rotation) for the complete
rotation and cleanup guidance.

`db.create_yaps(lobby_id, [{"id": "...", "data": ...}, ...])` atomically
creates 1–1000 Yaps in one engine process; duplicate IDs or an existing ID
reject the full batch. For pagination, `db.query_yap_page(lobby_id, options)`
returns `{"yaps": [...], "nextCursor": "..."}`. Pass `cursor` in the next
options or use `.after(cursor).execute_page()` on the query builder. Cursors
are bound to the project, Lobby, and filter/order options, not page size. Pagination is not a snapshot:
restart after project writes.

See the [SDK overview](https://github.com/pazti/Rizz-Base/blob/main/docs/sdks.md),
[conformance fixtures](https://github.com/pazti/Rizz-Base/blob/main/conformance/README.md),
and [runnable Node examples](https://github.com/pazti/Rizz-Base/blob/main/examples/README.md).
Protocol, query, security, and limit details are also covered in the
[Node.js SDK guide](https://github.com/pazti/Rizz-Base/blob/main/docs/node-sdk.md).
