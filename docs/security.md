# Security and limits

Rizzbase protects a file on your device. It does not protect you from yourself — read this once.

## Passphrase

- Choose a strong, unique passphrase. Get it from private input or a secrets manager.
- Never commit, log, print, or put it in CLI arguments. The SDK and CLI send it on stdin only.
- Do not expose the SDK through an untrusted server endpoint.
- There is **no passphrase recovery**. If you lose the passphrase, the encrypted Project is unreadable.
- Passphrase rotation is possible only when you still know the current passphrase: export the Project to a decrypted JSON backup, then import it into a new `.rizz` file using the new passphrase. This is a full decrypt-and-re-encrypt migration, not an in-place key change.

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
- The engine has Go fuzz targets for Brainrot encoding, envelope inspection, and Project JSON parsing. Storage tests kill a subprocess after its replacement file is synced but before publish, then verify the original encrypted file still decrypts; this validates the local-filesystem atomic-publish boundary, not arbitrary power-loss or network-filesystem behavior.

## Passphrase rotation

Rotation creates a new encrypted Project file; it does not change the encryption key in the existing file. The migration preserves the Project content and IDs, but the new file has newly generated encryption parameters.

1. Stop application writes to the source Project and make sure its latest data is present.
2. Open the source with its current passphrase and export a JSON backup to a protected location.
3. Create a new SDK instance for a different `.rizz` destination, supplying the new passphrase, then import the backup.
4. Open the new Project with the new passphrase and verify its contents before directing applications to the new path.
5. Keep the original encrypted file and a protected backup until verification is complete. Then remove the plaintext backup and retire old encrypted copies according to your backup-retention policy.

For example, in Node.js:

```ts
const source = new Rizzbase("./project.rizz", getCurrentPassphrasePrivately());
await source.export("./project-rotation.json");

const destination = new Rizzbase("./project-rotated.rizz", getNewPassphrasePrivately());
await destination.importBackup("./project-rotation.json");
await destination.open();
```

The Python SDK uses the same flow with `source.export(...)` and
`destination.import_backup(...)`. Exported JSON is plaintext and sensitive;
protect the backup directory and do not place it in source control, logs, or
shared temporary storage. Export and import refuse to overwrite existing
files. If import fails, the original encrypted Project remains unchanged.
Rotation cannot recover a Project if its current passphrase has already been
lost.

## Supply chain

- The release workflow rebuilds each platform asset independently with a pinned Go toolchain and requires byte-for-byte matches before publishing. It signs the SHA-256 manifest using keyless Sigstore Cosign/GitHub OIDC; verify the bundle and expected release workflow identity before trusting the manifest. The manifest then authenticates each listed asset by hash. The v1.2.0 release completed this workflow successfully.
- The install scripts currently verify SHA-256 but do not automatically verify the Cosign bundle. For publisher-authenticity checks, verify `SHA256SUMS.sigstore.json` with Cosign first, as shown in the [installation guide](./installation.md#manual-installation).
- Strongest policy: build the engine yourself with an explicit compatible version, inspect it, and set `binaryPath` to that executable. The SDK checks `--version` before use.
- Each engine request is capped at 64 KiB; query responses are bounded by the project file limit.

## V1 boundaries (non-negotiable)

Local-only. No server, realtime, multi-user, accounts, remote backup, schema enforcement, indexes, joins, or UI connection. The React screens on this site are mockups with sample data.
