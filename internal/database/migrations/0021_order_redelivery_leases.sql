-- Prevent repeated delivery requests from sending the same entitlement concurrently.
CREATE TABLE IF NOT EXISTS order_redelivery_leases (
  order_id TEXT PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
  lease_until TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
