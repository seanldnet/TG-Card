# Backup and restore (Docker Compose)

These scripts back up and restore both persistent data components used by the Compose deployment. New backups are encrypted with age (scrypt passphrase mode) before they are retained locally or delivered to Telegram:

- PostgreSQL database `tgcard` (custom-format `pg_dump` archive).
- Docker volume mounted at `/app/data` in the application container (`tg_card_data`), including the JWT signing key, encryption key and uploaded files.

## Encryption secret setup

Set the same externally managed secret on the deployment host in the uncommitted `.env` file before creating new backups:

```dotenv
BACKUP_ENCRYPTION_PASSPHRASE=replace-with-at-least-32-random-characters
```

Generate a high-entropy value (for example, `openssl rand -base64 48`) and store it in a password manager or separate secrets vault. **Do not store it in `/app/data`, the database, this repository, or inside the backup being encrypted.** Loss of this passphrase makes encrypted backups unrecoverable. Keep old passphrases until every backup encrypted with them has expired. Compose passes the value to the app container; recreate the app container after changing it.

If the variable is missing or shorter than 32 non-whitespace characters, scheduled backup and the local backup script fail closed rather than creating or delivering a plaintext backup. The scheduled retry mechanism continues to notify the Owner and retry after its normal cooldown.

## Create a backup

Run from the directory containing `docker-compose.yml`:

```bash
bash scripts/backup.sh
```

The script writes a timestamped directory under `./backups` and applies restrictive file permissions. Keep the backup directory off-host and encrypted according to your organization's retention policy. The default retention is seven local backup directories.

Optional environment variables:

- `BACKUP_DIR`: backup destination (default `./backups`).
- `BACKUP_KEEP`: number of newest backups to retain (default `7`, minimum `1`).

A successful new run produces `postgres.dump.age`, `tg_card_data.tar.gz.age`, and `SHA256SUMS`; plaintext intermediate files are removed before the backup directory is finalized. The checksum manifest detects accidental corruption, while age authenticated encryption protects confidentiality and integrity. The restore script verifies checksums and decrypts both components into a private temporary directory before any destructive operation. Legacy plaintext backup directories remain restorable for backward compatibility, but should be migrated to encrypted off-host storage. Store the complete directory together; restoring only one component can make encrypted credentials or uploaded assets unavailable.

## Restore a backup

**Restore is destructive.** It stops the application, overwrites the PostgreSQL database and replaces all contents of the application data volume. Take a fresh backup of the current system first. Do not run while orders, payments or procurement jobs are being processed.

Interactive restore:

```bash
bash scripts/restore.sh ./backups/tg-card-YYYYMMDDTHHMMSSZ
```

Non-interactive restore (explicit confirmation):

```bash
bash scripts/restore.sh ./backups/tg-card-YYYYMMDDTHHMMSSZ --yes
```

The database container remains online while the app is stopped. For age backups, the script requires the same `BACKUP_ENCRYPTION_PASSPHRASE`, verifies and decrypts the archives before stopping the app, then restores PostgreSQL, replaces `/app/data`, and starts the application. Afterward, verify `GET /api/health`, administrator login, Bot connectivity, payment watcher state, upstream status, recent orders and uploaded files before reopening sales.

## Operational notes

- Run backups on a host with Docker Compose access and enough free disk space for both archives.
- New local and scheduled backups are age-encrypted using the externally supplied `BACKUP_ENCRYPTION_PASSPHRASE`; the same secret is required for restore. The passphrase must remain outside the backed-up application data volume. Keep encrypted copies off-host as well.
- A backup is not verified until a restore has been tested against an isolated deployment.
- The scripts do not stop PostgreSQL during backup; `pg_dump` provides a transactionally consistent database snapshot. The application data volume is archived separately, so schedule backups during a quiet window for the strongest cross-component consistency.
- The default Compose project volume name is managed by Docker Compose; the scripts access it through the `app` service mount rather than guessing the generated Docker volume name.

## Automated isolated restore verification

The Go backend CI runs `bash scripts/test-backup-restore.sh` on an isolated Docker Compose project with disposable PostgreSQL 16 and application-data volumes. The test calls the same production backup and restore scripts; it does not use the production Compose project, production database, or production Docker volumes.

The test builds the backup encryption utility, uses a disposable test-only passphrase, and creates synthetic category, product, stock-card, order, and site-setting records plus representative persistent key files and an upload. It creates encrypted `.age` archives, asserts that plaintext intermediate archives are absent, changes the order state and setting, deletes the stock-card and upload, corrupts a key file, restores/decrypts the backup, and asserts that the order, inventory secret ciphertext, setting, key files, and upload all return to their pre-backup values. The test prints a simulated RPO window (time from successful backup completion to the simulated data-loss event) and the time from restore invocation until the isolated app container is running. The simulated RPO only measures the deliberately constructed CI scenario; it does not model a production backup schedule. The RTO is a test-container startup measurement only, not a production application readiness RTO.

A successful CI test proves the script workflow against the isolated fixture. It does not establish production RPO/RTO, off-host backup integrity, external passphrase escrow, or a real production restore. For production acceptance, record the backup timestamp, restore start/end timestamps, application health, login, Bot/payment/upstream checks, recent order consistency, and the image digest used. RPO depends on the interval and success of backups; RTO must be measured through application readiness and operational smoke tests, not merely container startup.


## Daily scheduled backup retry behavior

The application records the last successful backup date separately from the last attempt timestamp. A failed PostgreSQL dump or Telegram document delivery no longer marks the day as successfully backed up. The scheduler retains the failure timestamp, notifies the Owner, and retries after a 30-minute cooldown. Once a backup is delivered successfully, the local-date success marker is written and further scheduled attempts for that day are suppressed.

A notification or upload timeout can still leave the remote Telegram delivery outcome uncertain; verify the Owner chat before manually initiating another backup. CI coverage validates the retry decision logic, not a production restore. Continue to perform isolated restore drills and record production RPO/RTO separately.
