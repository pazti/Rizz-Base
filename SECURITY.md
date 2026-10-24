# Security policy

## Protecting project data

Rizzbase V1 stores all project metadata and Lobby/Yap data in encrypted
`.rizz` files. Brainrot encoding is reversible and is **not** a security layer.
The current key derivation is PBKDF2-HMAC-SHA256 with 600,000 iterations; weak
passphrases are still vulnerable to guessing. Use a strong, unique passphrase,
keep it private, and maintain separate encrypted backups.

Passphrases are supplied by the application through private stdin/API input.
Never put them in command-line arguments, source code, logs, issue reports, or
Git history. The library does not enforce minimum passphrase strength and
cannot recover a lost passphrase. If the current passphrase is known, it can
be rotated by exporting a decrypted JSON backup and importing it into a new
encrypted Project with a new passphrase. This is not an in-place key change;
see the [rotation procedure](docs/security.md#passphrase-rotation) and protect
the plaintext backup.

Local file access still matters: keep devices and backups secure. Project files
are created with owner-only permissions on Unix. A writer lock is a local
coordination mechanism, not protection from a hostile process running as the
same user. Do not use this V1 format for multi-user or network-hosted data.

## Reporting a vulnerability

Do not report exploitable details in a public issue. Contact the repository
owner privately through GitHub's security reporting feature if it is enabled.
Until a private reporting channel is configured, avoid including real project
files, passphrases, or sensitive user data in reports. Release binaries are
published on GitHub; there are no security response-time guarantees.
