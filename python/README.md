# Rizzbase Python SDK

Python 3.10+ SDK for the local Rizzbase engine. Install the engine separately;
the SDK never downloads a binary.

```sh
python -m pip install .
```

Set `RIZZBASE_BINARY` to the installed executable or leave it on `PATH`.
`binary_path` in the constructor takes precedence over both:

```python
from rizzbase import Rizzbase

db = Rizzbase("./my-project.rizz", get_passphrase_privately())
db.create("My project")

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

The synchronous API includes project creation/open, backup export/import,
lobby and Yap CRUD, and query filtering/ordering. Failures raise
`RizzbaseError` with a stable `code` and safe message. The SDK supports engine
versions `>=1.1.0 <2.0.0` and project format `1`; it checks compatibility once
per resolved binary path.

See [`docs/node-sdk.md`](../docs/node-sdk.md) for protocol, query, security and
limit details shared with the Node SDK.
