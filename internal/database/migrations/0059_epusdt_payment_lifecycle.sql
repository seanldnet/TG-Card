ALTER TABLE payment_settings ADD COLUMN epusdt_notify_url TEXT NOT NULL DEFAULT '';

CREATE TABLE epusdt_payment_intents (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    provider_trade_id TEXT NOT NULL DEFAULT '',
    payment_url TEXT NOT NULL DEFAULT '',
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'creating',
    expiration_time BIGINT NOT NULL DEFAULT 0,
    block_transaction_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_epusdt_payment_intents_trade_id
    ON epusdt_payment_intents(provider_trade_id)
    WHERE provider_trade_id <> '';

CREATE TABLE epusdt_payment_events (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    provider_trade_id TEXT NOT NULL UNIQUE,
    block_transaction_id TEXT NOT NULL DEFAULT '',
    amount_minor BIGINT NOT NULL,
    status TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);
