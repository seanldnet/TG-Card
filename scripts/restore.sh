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
BACKUP_ENCRYPTION_PASSPHRASE="${BACKUP_ENCRYPTION_PASSPHRASE:-}"
if [ ! -d "$BACKUP_DIR" ] || [ ! -f "$BACKUP_DIR/SHA256SUMS" ]; then
 echo "Backup directory must contain SHA256SUMS" >&2
 exit 2
fi
BACKUP_DIR="$(cd "$BACKUP_DIR" && pwd)"
(cd "$BACKUP_DIR" && sha256sum -c SHA256SUMS)

tmp_dir="$(mktemp -d)"
cleanup(){ rm -rf "$tmp_dir"; }
trap cleanup EXIT
backup_crypt() {
  local action="$1" input="$2" output="$3"
  if [ -n "${BACKUP_CRYPT_BIN:-}" ]; then
    BACKUP_ENCRYPTION_PASSPHRASE="$BACKUP_ENCRYPTION_PASSPHRASE" "$BACKUP_CRYPT_BIN" "$action" "$input" "$output"
  else
    docker compose run --rm --no-deps -T -v "$tmp_dir:/backup" -e BACKUP_ENCRYPTION_PASSPHRASE --entrypoint /app/backup-crypt app \
      "$action" "/backup/$(basename "$input")" "/backup/$(basename "$output")"
  fi
}

if [ -f "$BACKUP_DIR/postgres.dump.age" ] && [ -f "$BACKUP_DIR/tg_card_data.tar.gz.age" ]; then
  if [ "${#BACKUP_ENCRYPTION_PASSPHRASE}" -lt 32 ]; then
    echo "BACKUP_ENCRYPTION_PASSPHRASE must be configured to decrypt this backup" >&2
    exit 2
  fi
  echo "Encrypted age backup detected; decrypting into a private temporary directory..."
  cp -- "$BACKUP_DIR/postgres.dump.age" "$tmp_dir/"
  cp -- "$BACKUP_DIR/tg_card_data.tar.gz.age" "$tmp_dir/"
  backup_crypt decrypt "$tmp_dir/postgres.dump.age" "$tmp_dir/postgres.dump"
  backup_crypt decrypt "$tmp_dir/tg_card_data.tar.gz.age" "$tmp_dir/tg_card_data.tar.gz"
  DB_ARCHIVE="$tmp_dir/postgres.dump"
  DATA_ARCHIVE="$tmp_dir/tg_card_data.tar.gz"
elif [ -f "$BACKUP_DIR/postgres.dump" ] && [ -f "$BACKUP_DIR/tg_card_data.tar.gz" ]; then
  echo "Legacy plaintext backup detected. It will be restored for backward compatibility."
  DB_ARCHIVE="$BACKUP_DIR/postgres.dump"
  DATA_ARCHIVE="$BACKUP_DIR/tg_card_data.tar.gz"
else
  echo "Backup directory must contain both encrypted (.age) or legacy plaintext archive files" >&2
  exit 2
fi

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
docker compose exec -T db pg_restore -U tgcard -d tgcard --clean --if-exists --no-owner < "$DB_ARCHIVE"

echo "[3/5] Replacing /app/data volume contents..."
docker compose run --rm --no-deps -T --name "tg-card-restore-$(date -u +%Y%m%dT%H%M%SZ)" --entrypoint sh app -c 'for item in /app/data/* /app/data/.[!.]* /app/data/..?*; do [ -e "$item" ] || continue; rm -rf "$item"; done; tar -xzf - -C /app/data' < "$DATA_ARCHIVE"

echo "[4/5] Starting the application..."
docker compose up -d app
echo "[5/5] Restore complete. Verify application health and recent orders before resuming operations."
