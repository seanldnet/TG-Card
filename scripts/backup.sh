#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
BACKUP_DIR="${BACKUP_DIR:-$ROOT_DIR/backups}"
BACKUP_KEEP="${BACKUP_KEEP:-7}"
case "$BACKUP_KEEP" in ''|*[!0-9]*) echo "BACKUP_KEEP must be a positive integer" >&2; exit 2;; esac
if [ "$BACKUP_KEEP" -lt 1 ]; then echo "BACKUP_KEEP must be at least 1" >&2; exit 2; fi
if [ "${#BACKUP_ENCRYPTION_PASSPHRASE:-0}" -lt 32 ]; then
  echo "BACKUP_ENCRYPTION_PASSPHRASE must be configured with at least 32 characters; refusing plaintext backup" >&2
  exit 2
fi
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
tmp_dir="$(mktemp -d "$BACKUP_DIR/.tg-card-backup.XXXXXX")"
final_dir="$BACKUP_DIR/tg-card-$timestamp"
cleanup(){ [ ! -d "$tmp_dir" ] || rm -rf "$tmp_dir"; }
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

echo "[1/4] Dumping PostgreSQL database..."
docker compose exec -T db pg_dump -U tgcard -d tgcard --format=custom --no-owner > "$tmp_dir/postgres.dump"
test -s "$tmp_dir/postgres.dump"

echo "[2/4] Archiving the tg_card_data Docker volume..."
docker compose run --rm --no-deps -T --name "tg-card-backup-$timestamp" --entrypoint sh app -c 'tar -czf - -C /app/data .' > "$tmp_dir/tg_card_data.tar.gz"
test -s "$tmp_dir/tg_card_data.tar.gz"

echo "[3/4] Encrypting both backup components with age..."
backup_crypt encrypt "$tmp_dir/postgres.dump" "$tmp_dir/postgres.dump.age"
backup_crypt encrypt "$tmp_dir/tg_card_data.tar.gz" "$tmp_dir/tg_card_data.tar.gz.age"
rm -f "$tmp_dir/postgres.dump" "$tmp_dir/tg_card_data.tar.gz"
(cd "$tmp_dir" && sha256sum postgres.dump.age tg_card_data.tar.gz.age > SHA256SUMS)

cat > "$tmp_dir/manifest.txt" <<EOF
created_at_utc=$timestamp
database=PostgreSQL tgcard
volume=tg_card_data (/app/data)
repository=TG-Card
encryption=age scrypt; passphrase supplied externally
EOF
mv "$tmp_dir" "$final_dir"
trap - EXIT

echo "[4/4] Applying retention (keep newest $BACKUP_KEEP backups)..."
mapfile -t backups < <(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name 'tg-card-*' -printf '%f\n' | sort -r)
for ((i=BACKUP_KEEP; i<${#backups[@]}; i++)); do rm -rf -- "$BACKUP_DIR/${backups[$i]}"; done
echo "Encrypted backup completed: $final_dir"
