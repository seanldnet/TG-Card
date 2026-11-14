# Payment and delivery integration

## Current order and stock states

- `pending_payment`: one card key is reserved; its `product_keys.status` is `1`.
- `paid`: a trusted payment adapter has confirmed settlement; the reserved key is atomically changed to `status=2`.
- `delivery_failed`: payment remains final and the sold key is retained for retry.
- `delivered`: Telegram accepted the card delivery request and the order was finalized.
- `expired`: an unpaid order expired and its reserved key was released back to `status=0`.

## Trusted payment adapter contract

`database.ConfirmOrderPayment` is a database transition, not a payment verifier. Call it only after the provider or chain adapter has independently verified all of the following:

1. The transaction is confirmed according to the configured chain/provider finality policy.
2. The destination address matches the merchant's configured receiving address.
3. The received asset and network match the order's currency and configured payment rail.
4. The received amount meets the order amount and precision rules.
5. The transaction reference has not been applied to another order.

Pass a stable provider transaction reference to `ConfirmOrderPayment`. The database stores a unique payment reference and performs the order transition plus stock consumption in one transaction. Replaying the same method/reference is idempotent; a different reference cannot pay the same order again.

An order that has expired is not automatically accepted even if a late transfer is detected. The adapter must route late or underpaid transfers to a reconciliation/refund process rather than silently issuing a card.

## Delivery and recovery

After `ConfirmOrderPayment` succeeds, pass the returned `database.OrderDelivery` to `telegrambot.Manager.DeliverPaidOrder`. The manager decrypts the stored card key and sends it to the order's Telegram chat. It never writes plaintext card keys to logs.

If the process stops after payment confirmation, `database.GetOrderDelivery` can recover the paid card for orders in `paid` or `delivery_failed`. The authenticated admin endpoint `POST /api/orders/{id}/retry-delivery` invokes the same delivery path and records an audit event.

A background recovery loop also scans for due deliveries every 15 seconds. It claims up to 50 orders in a database transaction and sets a two-minute `delivery_lease_until` lease. PostgreSQL workers use `FOR UPDATE SKIP LOCKED`; SQLite uses its serialized write transaction. A worker restart leaves the order in `delivering`, and the next worker can reclaim it after the lease expires. Successful delivery clears the lease; failed delivery records the error and returns the order to `delivery_failed` for another scan. This lease prevents concurrent workers from normally sending the same order at the same time, but it does not remove Telegram's at-least-once delivery limitation described below.

Telegram `sendMessage` does not provide an idempotency key. If Telegram accepts a message but the network response is lost, a later retry can send the card again. The current delivery guarantee is therefore at-least-once, not exactly-once. The order number is included so duplicate messages can be recognized.

## TRC20 USDT watcher

The first live read-only payment rail is TRON mainnet USDT through the official TronGrid V1 account TRC-20 history endpoint:

- GET /v1/accounts/{address}/transactions/trc20
- only_confirmed=true and only_to=true
- Filtered by the configured USDT contract and paginated with TronGrid meta.fingerprint
- Requests authenticate with the backend-only TRON-PRO-API-KEY header

The watcher starts only when both TRONGRID_API_KEY and TRON_RECEIVE_ADDRESS are configured. TRON_USDT_CONTRACT defaults to the canonical TRON USDT contract TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t. The application never needs the receiving wallet private key because this integration only reads deposits.

Each new order receives a distinct six-decimal USDT invoice amount: the product two-decimal price plus a small random atomic-unit suffix. The database enforces uniqueness among active pending orders. The bot displays the exact amount and receiving address only when the watcher is enabled. Matching requires the exact amount and a block timestamp within the order creation/expiry window. The watcher then calls the idempotent payment transition and automatic Telegram delivery. Existing pending orders created before migration 0007 have no invoice amount and must be recreated; the bot warns users not to pay those legacy orders.

The watcher polls every 15 seconds and scans confirmed transfers from a fixed two-hour lookback window, using the same min_timestamp across all pages in one scan. It currently caps a scan at 25 pages (5,000 records). TronGrid only_confirmed=true is the confirmation gate; this implementation does not configure a separate N-confirmation threshold. Production API quotas and rate limits depend on the TronGrid key and plan.

Important operational limits:

- This is a single receiving-address / unique-amount design, not per-order wallet allocation.
- A transfer detected only after order expiry is not automatically credited, even if its block timestamp is close to expiry; it requires manual reconciliation.
- Unmatched, underpaid, overpaid, expired, or ambiguous transfers are not automatically refunded or credited. A dedicated transaction ledger and reconciliation UI are still future work.
- No wallet signing, sweeping, withdrawals, or refunds are implemented.
- GMpay and EPUSDT adapters are not implemented.

Configure the following in the server .env before enabling real payments:

~~~dotenv
TRONGRID_API_KEY=your-production-trongrid-key
TRON_RECEIVE_ADDRESS=your-merchant-tron-address
# Optional; defaults to the canonical TRON mainnet USDT contract.
TRON_USDT_CONTRACT=TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t
~~~

Keep the API key on the server only. Do not put it in frontend code, Telegram messages, or a public repository.

