# Append-only Events

Events store ordered activity such as analytics, logs, and audit records in the
encrypted Project file. They are separate from Lobbies and Yaps, and only
append/list/query operations are exposed: Events cannot be updated or deleted.

An Event contains a generated `id`, a required `kind`, optional `source`, an
engine-assigned `createdAt` timestamp, and arbitrary JSON `data`. Events are
returned in append order.

## SDK usage

```ts
await db.appendEvent("user.signup", { plan: "free" }, "web");
const page = await db.queryEvents({
  kind: "user.signup",
  source: "web",
  limit: 100,
});
console.log(page.events, page.nextCursor);
```

The Python methods are `append_event(kind, data, source="")`,
`list_events()`, and `query_events(options)`. In Go, use `AppendEvent`,
`ListEvents`, and `QueryEvents` with `EventQueryOptions`. All three SDKs use
the same engine operations and are covered by the shared
[conformance fixtures](../conformance/README.md).

`event.query` accepts optional exact-match `kind` and `source` filters, a
non-negative `limit`, and a `cursor` from the preceding page. Cursors are
bound to the project path and filters; changing either invalidates the cursor.
Appending new Events does not invalidate a cursor to an existing Event.

## Engine protocol

Append an Event:

```json
{
  "operation": "event.append",
  "path": "./journal.rizz",
  "passphrase": "provided privately",
  "kind": "user.signup",
  "source": "web",
  "data": { "plan": "free" }
}
```

Query Events with an `eventQuery` containing the filters and pagination
options. Success returns `eventPage.events` and `eventPage.nextCursor`.

New Projects include a generated optional `projectId`. Existing V1 Projects
without this metadata continue to open. Events are encrypted with all other
Project content and share the existing project-file size limit; because they
cannot be deleted, applications should account for file growth and export
backups as appropriate.
