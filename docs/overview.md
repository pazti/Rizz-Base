# What is Rizzbase?

Rizzbase is a **local-first database**. Each Project is one encrypted `.rizz` file on your computer. There is no cloud service, no network database, no account, and no realtime connection.

V1 ships three pieces:

- **Go engine + CLI** (`engine/`) — the source of truth. It encrypts, decrypts, and mutates `.rizz` files.
- **Node.js and Python SDKs** (`sdk/`, `python/`) — thin wrappers. Each spawns the engine once per operation over stdin/stdout.
- **Product UI** (this site) — a Coming Soon preview. Its dashboard screens are mockups and are **not** wired to your `.rizz` files.

## The vocabulary

| Word | Means | V1 representation |
| --- | --- | --- |
| Project | The database | One encrypted `.rizz` file |
| Lobby | A collection | Named group with a generated ID |
| Yap | A document | JSON value under an ID you choose |
| Field | A value | Any JSON value |

Think of it like this:

```text
my-project.rizz (Project)
├── general (Lobby, id: abc123)
│   ├── welcome (Yap)
│   └── rules (Yap)
└── users (Lobby, id: def456)
    └── mika (Yap)
```

- **Lobbies** get generated IDs from `createLobby`. Group related records, e.g. `users` or `messages`.
- **Yaps** use IDs you choose (1–128 bytes), unique per Lobby. Store plain JSON: objects, arrays, strings, numbers, booleans, `null`.
- Deleting a Lobby deletes all Yaps inside it in the same atomic write.

## How data flows

```text
Your code (SDK)
  │  JSON over stdin: { operation, path, passphrase, ... }
  ▼
Go engine (one process per operation)
  │  decrypt → mutate → encrypt → atomic rename
  ▼
./my-project.rizz (AES-256-GCM + Brainrot word encoding)
```

- The SDK never hosts a server. It calls `rizzbase` with one JSON object on stdin and reads one JSON object on stdout.
- Your passphrase travels on stdin only. Never in argv, never in env.
- Project writes use an adjacent `.lock` directory so two cooperating writers fail instead of corrupting each other.

## What V1 does not do

No server, realtime, or multi-user access. No accounts, hosted backups, schema enforcement, indexes, or joins. Backups are manual JSON exports you protect yourself. Queries decrypt and scan one Lobby in memory. The UI preview is not connected to the engine.

> Next: [install the engine](./installation.md), then use the [Node.js SDK](./node-sdk.md) or [Python SDK](../python/README.md).
