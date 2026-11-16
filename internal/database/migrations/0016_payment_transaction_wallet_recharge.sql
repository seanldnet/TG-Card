ALTER TABLE payment_transactions ADD COLUMN wallet_recharge_id TEXT;
CREATE INDEX IF NOT EXISTS idx_payment_transactions_wallet_recharge ON payment_transactions(wallet_recharge_id);
