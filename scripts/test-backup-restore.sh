#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
TMP_DIR="$(mktemp -d)"
PROJECT="tg-card-backup-test-$$"
export COMPOSE_PROJECT_NAME="$PROJECT"
export COMPOSE_FILE="$TMP_DIR/docker-compose.yml"
export POSTGRES_PASSWORD="isolated-backup-test-password"
BACKUP_DIR="$TMP_DIR/backups"
RESTORE_STARTED=0

cleanup() {
  docker compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cat > "$COMPOSE_FILE" <<'YAML'
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: tgcard
      POSTGRES_USER: tgcard
      POSTGRES_PASSWORD: isolated-backup-test-password
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U tgcard -d tgcard"]
      interval: 2s
      timeout: 2s
      retries: 30
    volumes:
      - postgres_test_data:/var/lib/postgresql/data
  app:
    image: alpine:3.21
    entrypoint: ["sh", "-c"]
    command: ["mkdir -p /app/data && sleep infinity"]
    volumes:
      - app_test_data:/app/data
    depends_on:
      db:
        condition: service_healthy
volumes:
  postgres_test_data:
  app_test_data:
YAML

echo "[1/6] Starting isolated PostgreSQL and application-data containers..."
docker compose up -d db app
for attempt in $(seq 1 60); do
  if docker compose exec -T db pg_isready -U tgcard -d tgcard >/dev/null 2>&1; then break; fi
  sleep 1
done
docker compose exec -T db pg_isready -U tgcard -d tgcard >/dev/null

echo "[2/6] Creating synthetic order/catalog/settings and persistent-key fixtures..."
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U tgcard -d tgcard <<'SQL'
CREATE TABLE backup_probe (id INTEGER PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO backup_probe(id, value) VALUES (1, 'before-backup');
SQL
docker compose exec -T app sh -c 'mkdir -p /app/data/uploads && printf "%s" "test-jwt-signing-key" > /app/data/jwt-secret && printf "%s" "test-encryption-key" > /app/data/bot-token.key && printf "%s" "original-upload" > /app/data/uploads/fixture.bin'

echo "[3/6] Running the real backup script against isolated Compose services..."
BACKUP_DIR="$BACKUP_DIR" BACKUP_KEEP=2 bash scripts/backup.sh
BACKUP_PATH="$(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name 'tg-card-*' | head -n 1)"
test -n "$BACKUP_PATH"

echo "[4/6] Mutating database and persistent files to simulate data loss..."
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U tgcard -d tgcard -c "UPDATE backup_probe SET value='after-backup' WHERE id=1; INSERT INTO backup_probe(id,value) VALUES(2,'must-disappear');"
docker compose exec -T app sh -c 'printf "%s" "corrupted-key" > /app/data/jwt-secret && rm -f /app/data/uploads/fixture.bin && printf "%s" "unexpected" > /app/data/extra.txt'

echo "[5/6] Restoring with the production restore script and measuring elapsed time..."
RESTORE_STARTED="$(date +%s)"
bash scripts/restore.sh "$BACKUP_PATH" --yes

echo "[6/6] Verifying database rows, key files, uploads, and running app container..."
VALUE="$(docker compose exec -T db psql -At -U tgcard -d tgcard -c "SELECT value FROM backup_probe WHERE id=1" | tr -d '\r')"
COUNT="$(docker compose exec -T db psql -At -U tgcard -d tgcard -c "SELECT COUNT(*) FROM backup_probe" | tr -d '\r')"
test "$VALUE" = "before-backup"
test "$COUNT" = "1"
docker compose exec -T app sh -c 'test "$(cat /app/data/jwt-secret)" = "test-jwt-signing-key" && test "$(cat /app/data/bot-token.key)" = "test-encryption-key" && test "$(cat /app/data/uploads/fixture.bin)" = "original-upload" && test ! -e /app/data/extra.txt'
docker compose ps --status running | grep -q 'app'
RESTORE_FINISHED="$(date +%s)"
echo "Isolated backup/restore integration test passed. Restore container-start RTO: $((RESTORE_FINISHED - RESTORE_STARTED)) seconds."
