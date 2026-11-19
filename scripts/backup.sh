#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
BACKUP_DIR="${BACKUP_DIR:-$ROOT_DIR/backups}"
BACKUP_KEEP="${BACKUP_KEEP:-7}"
case "$BACKUP_KEEP" in ''|*[!0-9]*) echo "BACKUP_KEEP must be a positive integer" >&2; exit 2;; esac
if [ "$BACKUP_KEEP" -lt 1 ]; then echo "BACKUP_KEEP must be at least 1" >&2; exit 2; fi
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
tmp_dir="$(mktemp -d "$BACKUP_DIR/.tg-card-backup.XXXXXX")"
final_dir="$BACKUP_DIR/tg-card-$timestamp"
cleanup(){ [ ! -d "$tmp_dir" ] || rm -rf "$tmp_dir"; }
trap cleanup EXIT

echo "[1/3] Dumping PostgreSQL database..."
docker compose exec -T db pg_dump -U tgcard -d tgcard --format=custom --no-owner > "$tmp_dir/postgres.dump"
test -s "$tmp_dir/postgres.dump"

echo "[2/3] Archiving the tg_card_data Docker volume..."
docker compose run --rm --no-deps -T --entrypoint sh app -c 'tar -czf - -C /app/data .' > "$tmp_dir/tg_card_data.tar.gz"
test -s "$tmp_dir/tg_card_data.tar.gz"

(cd "$tmp_dir" && sha256sum postgres.dump tg_card_data.tar.gz > SHA256SUMS)

cat > "$tmp_dir/manifest.txt" <<EOF
created_at_utc=$timestamp
database=PostgreSQL tgcard
volume=tg_card_data (/app/data)
repository=TG-Card
EOF
mv "$tmp_dir" "$final_dir"
trap - EXIT

echo "[3/3] Applying retention (keep newest $BACKUP_KEEP backups)..."
mapfile -t backups < <(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name 'tg-card-*' -printf '%f\n' | sort -r)
for ((i=BACKUP_KEEP; i<${#backups[@]}; i++)); do rm -rf -- "$BACKUP_DIR/${backups[$i]}"; done
echo "Backup completed: $final_dir"
