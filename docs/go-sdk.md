# Go SDK

The official Go SDK is a process client for the separately installed Rizzbase
engine. It uses the same JSON protocol as the Node.js and Python SDKs.

```sh
go get github.com/pazti/Rizz-Base/sdk/go
```

See the [Go SDK guide](../sdk/go/README.md) for installation, the full API, and
security guidance. The SDK provides context-aware project, Lobby, Yap, query,
and team-sync APIs.

## Append-only Events

Events are appended to the encrypted Project and cannot be updated or deleted.
The Go client supports appending, listing in append order, and cursor-paged
queries filtered by kind and source:

```go
event, err := db.AppendEvent(ctx, "user.signup", "web", map[string]any{
	"plan": "free",
})
events, err := db.ListEvents(ctx)

limit := 100
page, err := db.QueryEvents(ctx, rizzbase.EventQueryOptions{
	Kind: "user.signup",
	Limit: &limit,
})
```

Projects created by the engine also include a generated `projectId`. Older V1
projects without that optional metadata remain readable.
