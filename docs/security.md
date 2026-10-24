# Security and limits

Rizzbase protects a file on your device. It does not protect you from yourself — read this once.

## Passphrase

- Choose a strong, unique passphrase. Get it from private input or a secrets manager.
- Never commit, log, print, or put it in CLI arguments. The SDK and CLI send it on stdin only.
- Do not expose the SDK through an untrusted server endpoint.
- There is **no recovery and no key rotation**. Lose it and the file is unreadable.

## Encryption

- AES-256-GCM with PBKDF2-HMAC-SHA256 at 600,000 iterations, random 16-byte salt + 12-byte nonce per write.
- The header (version, salt, nonce, size) is public. Everything else is ciphertext + auth tag.
- Wrong passphrases and damaged ciphertext both fail authentication — by design you cannot tell them apart.
- Brainrot word-pair encoding is only a reversible representation. It adds zero secrecy.

## Files, locks, and backups

- One Project is one `.rizz` file, max 64 MiB encoded. Keep secure backups yourself.
- `export()` writes a decrypted JSON snapshot (whole Project, plain text, 64 MiB cap, owner-only permissions). Treat it as sensitive: store it securely, never commit it, delete it when done. Import validates the full structure first (`INVALID_BACKUP` on bad files); neither side overwrites existing files.
- Owner-only permissions on Unix. Never check `.rizz` files or passphrases into git.
- Writes serialize on an adjacent `.lock` directory with PID, host, start time, engine version, and a unique token in `lock.json`. A held lock returns `PROJECT_BUSY` with owner details when available.
- The `doctor` protocol operation checks file structure, format compatibility, lock ownership, temporary files, and permissions without modifying anything. Add a passphrase to decrypt and validate the project content.
- Lock owner state is `active`, `stale` (dead PID on this host), or `unknown` (remote host, missing/malformed metadata, or inconclusive process check). Diagnostics never remove locks. Confirm a writer cannot still run before manually removing a stale lock; do not treat `unknown` as stale.
- SDK writes wait up to the configured lock-wait budget for an active lock; stale locks are reported immediately. Writes are not replayed after timeout because the outcome may be ambiguous.
- New bytes are fully written + synced before atomic rename. The old file survives until publish. Network filesystems without atomic same-directory rename are unsupported.

## Supply chain

- Manual installs verify SHA-256 against the release `SHA256SUMS`. That detects corruption, not authorship — it is not a signature.
- Strongest policy: build the engine yourself with an explicit compatible version, inspect it, and set `binaryPath` to that executable. The SDK checks `--version` before use.
- Each engine request is capped at 64 KiB; query responses are bounded by the project file limit.

## V1 boundaries (non-negotiable)

Local-only. No server, realtime, multi-user, accounts, remote backup, schema enforcement, indexes, joins, or UI connection. The React screens on this site are mockups with sample data.
