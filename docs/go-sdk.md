# Go SDK

Rizzbase provides an official Go SDK through the `rizzbase/engine` module.
It uses the engine API in process, so it does not spawn a CLI process or
require a separately installed binary.

```sh
go get rizzbase/engine/core
```

Module: `rizzbase/engine`

```go
package main

import (
	"log"

	"rizzbase/engine/core"
)

func main() {
	project, err := core.CreateProject("./journal.rizz", "Personal journal", "private-passphrase")
	if err != nil {
		log.Fatal(err)
	}

	lobby, err := core.CreateLobby("./journal.rizz", "private-passphrase", "notes")
	if err != nil {
		log.Fatal(err)
	}

	err = core.CreateYap(
		"./journal.rizz",
		"private-passphrase",
		lobby.ID,
		"welcome",
		map[string]any{"message": "Hello, local storage"},
	)
	if err != nil {
		log.Fatal(err)
	}

	_ = project
}
```

## Core API

- `CreateProject`, `OpenProject`, and `ImportProject`
- `CreateLobby`, `ListLobbies`, and `DeleteLobby`
- `CreateYap`, `GetYap`, `ListYaps`, `UpdateYap`, and `DeleteYap`
- `QueryYaps` and `QueryYapPage`
- `InspectProjectBackup`
- `AppendEvent` and `ListEvents`

The Go SDK is the engine-native API and supports project format 1. Passphrases
are passed to Go functions directly; do not place passphrases in source control
or command-line arguments.

## Queries

```go
limit := 10
yaps, err := core.QueryYaps("./journal.rizz", "private-passphrase", lobby.ID,
	core.QueryOptions{
		Limit: &limit,
		Where: []core.Filter{{
			Field:    "message",
			Operator: "==",
			Value:    []byte(`"Hello, local storage"`),
		}},
	})
```

Go uses `json.RawMessage` for filter values and Yap data. Use cursor pagination
with `QueryYapPage` and `QueryPage.NextCursor`.

## Compatibility

The official Go SDK follows the engine's major-version compatibility line:
engine major versions below 2 remain in the V1 format. Project format migrations
require an explicit compatibility decision and conformance coverage.
