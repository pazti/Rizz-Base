# Rizzbase examples

These small interactive Node.js apps demonstrate the SDK against local
encrypted Project files:

- [`notes.mjs`](./notes.mjs): add notes, list them, or search for an exact title.
- [`todos.mjs`](./todos.mjs): add, list, and complete todos.

## Run from this repository

The published `rizzbase@1.1.0` includes these features. The example scripts
import the SDK from this repository, so build the local SDK and engine to run
them against the checked-out source:

```sh
cd sdk
npm install
npm run build
cd ../engine
go build -ldflags="-X main.version=1.1.0" -o /tmp/rizzbase-engine ./cmd/rizzbase
cd ..
```

Set a private passphrase and run either app. Do not put a real passphrase in
source code, command arguments, or shell history. Prefer your operating
system's secret manager. The prompt below reads it without echoing it into
terminal output or command history (Bash/Zsh):

```sh
read -rsp "Project passphrase: " RIZZBASE_PASSPHRASE
printf '\n'
export RIZZBASE_PASSPHRASE
export RIZZBASE_BINARY=/tmp/rizzbase-engine
node examples/notes.mjs
# or
node examples/todos.mjs
```

By default, encrypted `.rizz` files are stored in
`~/.local/share/rizzbase/examples/`. Set `RIZZBASE_EXAMPLES_DIR` to use another
directory (for example, a temporary directory during testing). The examples
create their Projects on first run and reopen them on later runs. They do not
overwrite existing Project files.

The SDK also supports `db.export("./backup.json")` and
`db.importBackup("./backup.json")`. Projects and JSON backups are limited to
64 MiB. Exported JSON is decrypted plaintext and must be protected; export and
import refuse to overwrite existing files. Import into a new `.rizz` path.
