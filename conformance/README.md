# Cross-SDK conformance

Fixtures in `fixtures/` are language-neutral JSON scenario files. Each fixture
contains ordered engine operations, optional setup files, captured values, and
expected response fields or error codes. Paths and generated IDs are isolated
per run; runners compare only stable values, not timestamps, generated IDs, or
absolute paths.

## Run locally

Build a compatible engine and the Node SDK, then run both adapters against the
same fixtures:

```sh
cd engine
go build -ldflags="-X main.version=1.1.0" -o /tmp/rizzbase ./cmd/rizzbase
go build -ldflags="-X main.version=2.0.0" -o /tmp/rizzbase-incompatible ./cmd/rizzbase
go build -o /tmp/rizzbase-protocol-error ./cmd/conformance-test-engine
go build -ldflags="-X main.mode=nonzero" -o /tmp/rizzbase-nonzero ./cmd/conformance-test-engine
cd ../sdk
npm install
npm run build
cd ../python
python3 -m pip install .
cd ..
export RIZZBASE_BINARY=/tmp/rizzbase
export RIZZBASE_INCOMPATIBLE_BINARY=/tmp/rizzbase-incompatible
export RIZZBASE_PROTOCOL_ERROR_BINARY=/tmp/rizzbase-protocol-error
export RIZZBASE_NONZERO_BINARY=/tmp/rizzbase-nonzero
node conformance/node-runner.cjs
python3 conformance/python-runner.py
```

The Node and Python runners call their production SDKs. Both adapters normalize
timestamps, generated IDs, and machine-specific paths before producing
per-step results.

CI runs both runners and compares their normalized results. Add scenario
coverage by extending fixtures rather than duplicating expectations in
language-specific tests. Keep fixture inputs offline and avoid assertions on
timestamps, generated IDs, and machine-specific file paths.
