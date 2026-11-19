#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
 echo "Usage: bash scripts/restore.sh <backup-directory> [--yes]" >&2
 exit 2
fi
BACKUP_DIR="$1"
CONFIRM="${2:-}"
if [ ! -f "$BACKUP_DIR/postgres.dump" ] || [ ! -f "$BACKUP_DIR/tg_card_data.tar.gz" ] || [ ! -f "$BACKUP_DIR/SHA256SUMS" ]; then
 echo "Backup directory must contain postgres.dump and tg_card_data.tar.gz" >&2
 exit 2
fi
(cd "$BACKUP_DIR" && sha256sum -c SHA256SUMS)
if [ "$CONFIRM" != "--yes" ]; then
 echo "WARNING: this will stop the application and OVERWRITE the current PostgreSQL database and /app/data volume."
 echo "Backup: $BACKUP_DIR"
 read -r -p "Type RESTORE to continue: " answer
 [ "$answer" = "RESTORE" ] || { echo "Restore cancelled."; exit 1; }
fi

echo "[1/5] Stopping the application and ensuring PostgreSQL is online..."
docker compose stop app
docker compose up -d db
for attempt in $(seq 1 30); do
 if docker compose exec -T db pg_isready -U tgcard -d tgcard >/dev/null 2>&1; then break; fi
 sleep 2
done
docker compose exec -T db pg_isready -U tgcard -d tgcard >/dev/null

echo "[2/5] Restoring PostgreSQL database..."
docker compose exec -T db pg_restore -U tgcard -d tgcard --clean --if-exists --no-owner < "$BACKUP_DIR/postgres.dump"

echo "[3/5] Replacing /app/data volume contents..."
docker compose run --rm --no-deps -T --name "tg-card-restore-$(date -u +%Y%m%dT%H%M%SZ)" --entrypoint sh app -c 'for item in /app/data/* /app/data/.[!.]* /app/data/..?*; do [ -e "$item" ] || continue; rm -rf "$item"; done; tar -xzf - -C /app/data' < "$BACKUP_DIR/tg_card_data.tar.gz"

echo "[4/5] Starting the application..."
docker compose up -d app
echo "[5/5] Restore complete. Verify application health and recent orders before resuming operations."
