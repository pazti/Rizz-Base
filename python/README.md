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

See the [SDK overview](https://github.com/pazti/Rizz-Base/blob/main/docs/sdks.md),
[conformance fixtures](https://github.com/pazti/Rizz-Base/blob/main/conformance/README.md),
and [runnable Node examples](https://github.com/pazti/Rizz-Base/blob/main/examples/README.md).
Protocol, query, security, and limit details are also covered in the
[Node.js SDK guide](https://github.com/pazti/Rizz-Base/blob/main/docs/node-sdk.md).
