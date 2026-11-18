# Go SDK

The Go SDK is a process client for the separately installed Rizzbase engine.
It uses the same JSON-over-stdin/stdout protocol as the Node.js and Python
SDKs. It never downloads the engine.

## Install

```sh
go get github.com/pazti/Rizz-Base/sdk/go
```

Install the engine separately and put `rizzbase` on `PATH`, set
`RIZZBASE_BINARY`, or provide an explicit trusted executable path.

## Quick start

```go
package main

import (
	"context"
	"errors"
	"log"

	rizzbase "github.com/pazti/Rizz-Base/sdk/go"
)

func main() {
	ctx := context.Background()
	db, err := rizzbase.New("./journal.rizz", readPassphrasePrivately(),
		rizzbase.WithBinaryPath("/path/to/rizzbase"),
	)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := db.Open(ctx); err != nil {
		var sdkErr *rizzbase.Error
		if !errors.As(err, &sdkErr) || sdkErr.Code != rizzbase.CodeFileNotFound {
			log.Fatal(err)
		}
		if _, err := db.Create(ctx, "Personal journal"); err != nil {
			log.Fatal(err)
		}
	}

	lobby, err := db.CreateLobby(ctx, "notes")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := db.CreateYap(ctx, lobby.ID, "hello", map[string]any{
		"title": "Hello, local storage",
		"score": 10,
	}); err != nil {
		log.Fatal(err)
	}

	page, err := db.Query(lobby.ID).
		Where("score", rizzbase.GreaterEqual, 5).
		OrderBy("score", rizzbase.Descending).
		Limit(20).
		ExecPage(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(len(page.Yaps))

	if err := db.Export(ctx, "./backup.json"); err != nil {
		log.Fatal(err)
	}
}
```

`readPassphrasePrivately` is application code; use a private prompt or secret
manager. Never hardcode, log, or pass the passphrase in process arguments.

## API

Create a client with `New(path, passphrase, options...)`; project paths must
end in `.rizz`. All engine operations take a `context.Context`.

- Project: `Create`, `Open`, `Doctor`, `Export`, `ImportBackup`
- Lobby: `CreateLobby`, `ListLobbies`, `DeleteLobby`
- Yap: `CreateYap`, `CreateYaps` (atomic batches of 1–1000), `GetYap`,
  `ListYaps`, `UpdateYap`, `DeleteYap`
- Queries: `QueryYaps`, `QueryYapPage`, and immutable `Query` builder with
  `Where`, `OrderBy`, `Limit`, `After`, `Exec`, and `ExecPage`

`GetYap` returns `json.RawMessage`; `Yap.Data` is also raw JSON so callers can
decode into their own types. Creation and update methods accept ordinary Go
values encodable by `encoding/json`.

Use `WithBinaryPath`, `WithTimeout`, and `WithRetry` to configure the client.
Binary resolution is explicit path → `RIZZBASE_BINARY` → `PATH`. Before the
first request to each binary, the SDK validates engine version `>=1.1.0 <2.0.0`
and project format `1`. Only safe read timeouts are retried; writes are not
replayed, and wait for locks only within the configured `LockWait`.

`WithRetry` takes a complete `RetryOptions` value. Defaults are 3 attempts,
75 ms base delay, 1 s maximum delay, a 2 s additional retry budget, and a 1 s
write lock wait (each budget is reduced automatically when `WithTimeout` is
shorter).

Errors are `*rizzbase.Error` values with stable `Code` constants. Busy-project
details are available in `Context`. Exported JSON backups are plaintext,
owner-only, limited to 64 MiB, and never overwrite an existing destination.

## Conformance and tests

From the repository root, build the test engines and run the shared fixture
adapters as described in [`conformance/README.md`](../../conformance/README.md).
The Go adapter is `go run ./cmd/conformance` from this module; it accepts
optional fixture paths and validates expected fields and error codes before
emitting normalized results. `go test -race ./...` also tests read-timeout
retry behavior, write non-replay, active-lock waiting, stale-lock reporting,
and the full engine lifecycle.
