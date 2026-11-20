# Cross-SDK conformance

Fixtures in `fixtures/` are language-neutral JSON scenario files. Each fixture
contains ordered engine operations, optional setup files, captured values, and
expected response fields or error codes. Paths and generated IDs are isolated
per run; runners compare only stable values, not timestamps, generated IDs, or
absolute paths.

## Run locally

Build a compatible engine and the language SDKs, then run their adapters
against the same fixtures. Each adapter runs the production SDK; compare their
normalized results after all three runs:

```sh
cd engine
go build -ldflags="-X main.version=1.1.0" -o /tmp/rizzbase ./cmd/rizzbase
go build -ldflags="-X main.version=2.0.0" -o /tmp/rizzbase-incompatible ./cmd/rizzbase
go build -o /tmp/rizzbase-protocol-error ./cmd/conformance-test-engine
go build -ldflags="-X main.mode=nonzero" -o /tmp/rizzbase-nonzero ./cmd/conformance-test-engine
export RIZZBASE_BINARY=/tmp/rizzbase
export RIZZBASE_INCOMPATIBLE_BINARY=/tmp/rizzbase-incompatible
export RIZZBASE_PROTOCOL_ERROR_BINARY=/tmp/rizzbase-protocol-error
export RIZZBASE_NONZERO_BINARY=/tmp/rizzbase-nonzero
cd ../sdk
npm install
npm run build
cd ../python
python3 -m pip install .
cd ../sdk/go
go test -race ./...
go run ./cmd/conformance > /tmp/go-conformance.json
cd ../..
node conformance/node-runner.cjs > /tmp/node-conformance.json
python3 conformance/python-runner.py > /tmp/python-conformance.json
python3 conformance/compare.py /tmp/node-conformance.json /tmp/python-conformance.json /tmp/go-conformance.json
```

The Node, Python, and Go runners normalize timestamps, generated IDs, and
machine-specific paths before producing per-step results. The Go runner lives
in [`sdk/go/cmd/conformance/`](../sdk/go/cmd/conformance/) and accepts optional
fixture paths; without arguments it runs every JSON fixture.

CI runs all three runners and compares their normalized JSON results. Add
scenario coverage by extending fixtures rather than duplicating expectations
in language-specific tests. Keep fixture inputs offline and avoid assertions
on timestamps, generated IDs, and machine-specific file paths.

The query fixture covers atomic `yap.bulkCreate` and `yap.query` cursor
continuation, ensuring all SDKs share the batch and page response shapes. The
Events fixture covers append order, exact kind/source filters, cursor
continuation, and rejection of cursors used with different filters.
The sync-journal fixture initializes owner membership, journals normal Lobby
and Yap writes, and compares the resulting membership, operation, and conflict
counts across all three SDKs. Sync exchange remains application-provided; the
fixture does not imply a network transport.
