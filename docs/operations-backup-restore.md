# Backup and restore (Docker Compose)

These scripts back up and restore both persistent data components used by the Compose deployment:

- PostgreSQL database `tgcard` (custom-format `pg_dump` archive).
- Docker volume mounted at `/app/data` in the application container (`tg_card_data`), including the JWT signing key, encryption key and uploaded files.

## Create a backup

Run from the directory containing `docker-compose.yml`:

```bash
bash scripts/backup.sh
```

The script writes a timestamped directory under `./backups` and applies restrictive file permissions. Keep the backup directory off-host and encrypted according to your organization's retention policy. The default retention is seven local backup directories.

Optional environment variables:

- `BACKUP_DIR`: backup destination (default `./backups`).
- `BACKUP_KEEP`: number of newest backups to retain (default `7`, minimum `1`).

A successful run must produce `postgres.dump`, `tg_card_data.tar.gz`, and `SHA256SUMS`. The restore script verifies both archives against the checksum manifest before any destructive operation. Store the complete directory together; restoring only one component can make encrypted credentials or uploaded assets unavailable.

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

The database container remains online while the app is stopped. The script restores PostgreSQL, replaces `/app/data`, then starts the application. Afterward, verify `GET /api/health`, administrator login, Bot connectivity, payment watcher state, upstream status, recent orders and uploaded files before reopening sales.

## Operational notes

- Run backups on a host with Docker Compose access and enough free disk space for both archives.
- The PostgreSQL archive and app data archive may contain sensitive business data and cryptographic keys. Restrict access and encrypt off-host copies.
- A backup is not verified until a restore has been tested against an isolated deployment.
- The scripts do not stop PostgreSQL during backup; `pg_dump` provides a transactionally consistent database snapshot. The application data volume is archived separately, so schedule backups during a quiet window for the strongest cross-component consistency.
- The default Compose project volume name is managed by Docker Compose; the scripts access it through the `app` service mount rather than guessing the generated Docker volume name.

## Automated isolated restore verification

The Go backend CI runs `bash scripts/test-backup-restore.sh` on an isolated Docker Compose project with disposable PostgreSQL 16 and application-data volumes. The test calls the same production backup and restore scripts; it does not use the production Compose project, production database, or production Docker volumes.

The test creates a synthetic database row plus representative persistent key files and an upload, creates a backup, mutates/deletes the originals, restores the backup, and asserts that the original database value and files return while post-backup mutations disappear. The test prints the time from restore invocation until the isolated app container is running. This is a test-container startup measurement only, not a production application readiness RTO.

A successful CI test proves the script workflow against the isolated fixture. It does not establish production RPO/RTO, off-host backup integrity, encryption-at-rest, or a real production restore. For production acceptance, record the backup timestamp, restore start/end timestamps, application health, login, Bot/payment/upstream checks, recent order consistency, and the image digest used. RPO depends on the interval and success of backups; RTO must be measured through application readiness and operational smoke tests, not merely container startup.
