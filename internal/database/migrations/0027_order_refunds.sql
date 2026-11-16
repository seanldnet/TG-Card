-- Record internal wallet refunds and support revocation of delivered card stock.
ALTER TABLE orders ADD COLUMN refunded_at TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN refund_note TEXT NOT NULL DEFAULT '';
