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
export BACKUP_ENCRYPTION_PASSPHRASE="isolated-ci-backup-passphrase-with-more-than-32-chars"
export BACKUP_CRYPT_BIN="$TMP_DIR/backup-crypt"
go build -o "$BACKUP_CRYPT_BIN" ./cmd/backup-crypt
printf "%s" "preflight" > "$TMP_DIR/preflight.txt"
if BACKUP_ENCRYPTION_PASSPHRASE="short" "$BACKUP_CRYPT_BIN" encrypt "$TMP_DIR/preflight.txt" "$TMP_DIR/should-not-exist.age"; then
  echo "backup encryption unexpectedly accepted a weak passphrase" >&2
  exit 1
fi
test ! -e "$TMP_DIR/should-not-exist.age"
rm -f "$TMP_DIR/preflight.txt"
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
CREATE TABLE product_categories (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, sort_order INTEGER NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE products (
  id TEXT PRIMARY KEY, category_id TEXT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
  price_minor INTEGER NOT NULL DEFAULT 0, currency TEXT NOT NULL DEFAULT 'USDT',
  delivery_type TEXT NOT NULL DEFAULT 'card', status INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  FOREIGN KEY(category_id) REFERENCES product_categories(id) ON DELETE SET NULL
);
CREATE TABLE product_keys (
  id TEXT PRIMARY KEY, product_id TEXT NOT NULL, secret_ciphertext TEXT NOT NULL,
  secret_fingerprint TEXT NOT NULL, status INTEGER NOT NULL DEFAULT 0, order_id TEXT,
  created_at TEXT NOT NULL, sold_at TEXT,
  FOREIGN KEY(product_id) REFERENCES products(id) ON DELETE CASCADE
);
CREATE TABLE orders (
  id TEXT PRIMARY KEY, order_no TEXT NOT NULL UNIQUE, bot_id TEXT NOT NULL DEFAULT '',
  telegram_user_id TEXT NOT NULL, telegram_chat_id TEXT NOT NULL, product_id TEXT,
  product_name TEXT NOT NULL, unit_price_minor INTEGER NOT NULL, quantity INTEGER NOT NULL DEFAULT 1,
  amount_minor INTEGER NOT NULL, currency TEXT NOT NULL DEFAULT 'USDT',
  status TEXT NOT NULL DEFAULT 'pending_payment', expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL, paid_at TEXT, delivered_at TEXT,
  FOREIGN KEY(product_id) REFERENCES products(id) ON DELETE SET NULL
);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);
INSERT INTO product_categories(id,name,created_at,updated_at) VALUES ('cat-1','Backup Test','2026-01-01','2026-01-01');
INSERT INTO products(id,category_id,name,price_minor,created_at,updated_at) VALUES ('product-1','cat-1','Synthetic Card',100,'2026-01-01','2026-01-01');
INSERT INTO product_keys(id,product_id,secret_ciphertext,secret_fingerprint,created_at) VALUES ('stock-1','product-1','encrypted-stock-secret','fingerprint-1','2026-01-01');
INSERT INTO orders(id,order_no,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,amount_minor,status,expires_at,created_at,updated_at,paid_at)
VALUES ('order-1','TG-BACKUP-ORDER-1','user-1','chat-1','product-1','Synthetic Card',100,100,'paid','2026-01-02','2026-01-01','2026-01-01','2026-01-01');
INSERT INTO settings(key,value,updated_at) VALUES ('cap_enabled','true','2026-01-01');
SQL
docker compose exec -T app sh -c 'mkdir -p /app/data/uploads && printf "%s" "test-jwt-signing-key" > /app/data/jwt-secret && printf "%s" "test-encryption-key" > /app/data/bot-token.key && printf "%s" "original-upload" > /app/data/uploads/fixture.bin'

echo "[3/6] Running the real backup script against isolated Compose services..."
BACKUP_DIR="$BACKUP_DIR" BACKUP_KEEP=2 bash scripts/backup.sh
BACKUP_FINISHED="$(date +%s)"
BACKUP_PATH="$(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name 'tg-card-*' | head -n 1)"
test -n "$BACKUP_PATH"
test -f "$BACKUP_PATH/postgres.dump.age"
test -f "$BACKUP_PATH/tg_card_data.tar.gz.age"
test ! -e "$BACKUP_PATH/postgres.dump"
test ! -e "$BACKUP_PATH/tg_card_data.tar.gz"
if grep -a -q "test-encryption-key" "$BACKUP_PATH/tg_card_data.tar.gz.age"; then echo "encrypted backup unexpectedly contains plaintext key" >&2; exit 1; fi

echo "[4/6] Mutating database and persistent files to simulate data loss..."
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U tgcard -d tgcard -c "UPDATE orders SET status='refunded' WHERE id='order-1'; DELETE FROM product_keys WHERE id='stock-1'; UPDATE settings SET value='false' WHERE key='cap_enabled';"
docker compose exec -T app sh -c 'printf "%s" "corrupted-key" > /app/data/jwt-secret && rm -f /app/data/uploads/fixture.bin && printf "%s" "unexpected" > /app/data/extra.txt'
SIMULATED_FAILURE_AT="$(date +%s)"

echo "[5/7] Verifying a wrong passphrase cannot begin destructive restore..."
if BACKUP_ENCRYPTION_PASSPHRASE="wrong-ci-passphrase-that-is-long-enough-to-pass-validation" bash scripts/restore.sh "$BACKUP_PATH" --yes; then
  echo "restore unexpectedly accepted the wrong passphrase" >&2
  exit 1
fi
ORDER_STATUS="$(docker compose exec -T db psql -At -U tgcard -d tgcard -c "SELECT status FROM orders WHERE order_no='TG-BACKUP-ORDER-1'" | tr -d '\r')"
test "$ORDER_STATUS" = "refunded"
docker compose ps --status running | grep -q 'app'

echo "[6/7] Restoring with the production restore script and measuring elapsed time..."
RESTORE_STARTED="$(date +%s)"
bash scripts/restore.sh "$BACKUP_PATH" --yes

echo "[7/7] Verifying database rows, key files, uploads, and running app container..."
ORDER_STATUS="$(docker compose exec -T db psql -At -U tgcard -d tgcard -c "SELECT status FROM orders WHERE order_no='TG-BACKUP-ORDER-1'" | tr -d '\r')"
STOCK_COUNT="$(docker compose exec -T db psql -At -U tgcard -d tgcard -c "SELECT COUNT(*) FROM product_keys WHERE id='stock-1' AND secret_ciphertext='encrypted-stock-secret'" | tr -d '\r')"
CAP_ENABLED="$(docker compose exec -T db psql -At -U tgcard -d tgcard -c "SELECT value FROM settings WHERE key='cap_enabled'" | tr -d '\r')"
test "$ORDER_STATUS" = "paid"
test "$STOCK_COUNT" = "1"
test "$CAP_ENABLED" = "true"
docker compose exec -T app sh -c 'test "$(cat /app/data/jwt-secret)" = "test-jwt-signing-key" && test "$(cat /app/data/bot-token.key)" = "test-encryption-key" && test "$(cat /app/data/uploads/fixture.bin)" = "original-upload" && test ! -e /app/data/extra.txt'
docker compose ps --status running | grep -q 'app'

echo "[7/7] Verifying backward compatibility with legacy plaintext backups..."
LEGACY_BACKUP="$BACKUP_DIR/legacy-plaintext"
mkdir -p "$LEGACY_BACKUP"
"$BACKUP_CRYPT_BIN" decrypt "$BACKUP_PATH/postgres.dump.age" "$LEGACY_BACKUP/postgres.dump"
"$BACKUP_CRYPT_BIN" decrypt "$BACKUP_PATH/tg_card_data.tar.gz.age" "$LEGACY_BACKUP/tg_card_data.tar.gz"
(cd "$LEGACY_BACKUP" && sha256sum postgres.dump tg_card_data.tar.gz > SHA256SUMS)
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U tgcard -d tgcard -c "UPDATE orders SET status='refunded' WHERE id='order-1';"
bash scripts/restore.sh "$LEGACY_BACKUP" --yes
ORDER_STATUS="$(docker compose exec -T db psql -At -U tgcard -d tgcard -c "SELECT status FROM orders WHERE order_no='TG-BACKUP-ORDER-1'" | tr -d '\r')"
test "$ORDER_STATUS" = "paid"

RESTORE_FINISHED="$(date +%s)"
echo "Encrypted and legacy backup/restore integration test passed. Simulated data-loss window (RPO): $((SIMULATED_FAILURE_AT - BACKUP_FINISHED)) seconds. Restore-to-container-running RTO: $((RESTORE_FINISHED - RESTORE_STARTED)) seconds."
