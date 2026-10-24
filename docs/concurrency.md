# Concurrency contract

Rizzbase stores one Project in one local `.rizz` file. Its concurrency guarantees
apply to processes using the same local filesystem; network filesystems and
multi-host coordination are not supported.

## Reads and writes

- Reads do not acquire the write lock. Atomic same-directory replacement means
  a reader sees a complete old or new file, not a partially written project.
- Every mutation acquires the adjacent `<project>.rizz.lock` directory before
  reading the project and holds it through atomic publication.
- Lock acquisition is exclusive across SDK instances and engine processes that
  share the filesystem and honor this protocol.
- Calls made through one Node SDK instance are serialized in call order. The
  synchronous Python SDK serializes calls made through one instance. These
  instance queues do not coordinate other instances or external processes;
  the engine lock provides that coordination.
- A write with `lockWaitMs` waits for a known active local writer only for the
  requested bounded interval. A stale or unknown lock returns immediately as
  `PROJECT_BUSY`; the engine never deletes it automatically.
- If a write times out, the SDK does not replay it: the operation may already
  have committed. The caller must inspect state before deciding whether to
  repeat an application-level action.

## Lock diagnosis

`doctor` reports a lock as:

- `active`: metadata identifies a live PID on this host;
- `stale`: metadata identifies a PID that is no longer running on this host;
- `unknown`: metadata is malformed/unavailable, the host differs, or the
  process state cannot be established.

Only `stale` is a positive dead-process observation, not authorization to
delete automatically. Confirm the process cannot be restarted and the lock
still belongs to the expected project before manual removal. Never treat
`unknown` as stale.

## Scope

This is single-host file locking, not a transaction service, distributed lock,
or multi-user database. Do not share a project over a network filesystem or
expect a process that bypasses the engine protocol to honor the lock.
