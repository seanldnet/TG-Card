# Payment and delivery integration

## Current order and stock states

- `pending_payment`: one card key is reserved; its `product_keys.status` is `1`.
- `paid`: a trusted payment adapter has confirmed settlement; the reserved key is atomically changed to `status=2`.
- `delivery_failed`: payment remains final and the sold key is retained for retry.
- `delivered`: Telegram accepted the card delivery request and the order was finalized.
- `expired`: an unpaid order expired and its reserved key was released back to `status=0`.
- `purchase_failed`: a paid upstream order could not be procured or queried before its configured timeout; the original payment reference remains intact and the buyer's wallet is unchanged.

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

After `ConfirmOrderPayment` succeeds, the immediate payment path and admin manual-retry endpoint first acquire the order's two-minute delivery lease with `database.ClaimOrderDelivery`. The background worker uses `database.ClaimOrderDeliveries` for batches. Only the lease owner loads the paid card with `database.GetOrderDelivery` and calls `telegrambot.Manager.DeliverPaidOrder`. This prevents the payment watcher, background worker, and admin retry action from dispatching the same order concurrently. The manager decrypts the stored card key and sends it to the order's Telegram chat. It never writes plaintext card keys to logs.

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

Each new order receives a distinct six-decimal USDT invoice amount: the product two-decimal price plus a small random atomic-unit suffix. The database enforces uniqueness among active pending orders. The bot displays the exact amount and receiving address only when the watcher is enabled. Matching requires the exact amount and a block timestamp within the order creation/expiry window. The watcher calls the idempotent payment transition, acquires the same delivery lease used by the background queue, and only then attempts immediate Telegram delivery. If another worker already owns the lease, that worker remains responsible for dispatch. Existing pending orders created before migration 0007 have no invoice amount and must be recreated; the bot warns users not to pay those legacy orders.

The watcher polls every 15 seconds and scans confirmed transfers from a fixed two-hour lookback window, newest first, using the same min_timestamp and max_timestamp across all pages in one scan. It currently caps a scan at 25 pages (5,000 records); reaching that cap is logged so operators know older records may need a deeper reconciliation scan. TronGrid only_confirmed=true is the confirmation gate; this implementation does not configure a separate N-confirmation threshold. Production API quotas and rate limits depend on the TronGrid key and plan.

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



## Administrator-managed TRC20 configuration

TronGrid API credentials and the receiving address are managed from **Admin → 支付设置**. The API key is encrypted with the application's persistent AES-256-GCM secret key before it is stored in the database. The API never returns the plaintext key; the page only reports whether one is configured. Leaving the API-key field empty preserves the existing key, while the explicit clear option removes it.

Saving the page updates the live payment watcher and Telegram bot checkout details without restarting the service. The settings are stored in the same SQLite/PostgreSQL database as orders and survive container recreation and image upgrades. .env is no longer required for payment setup. Existing installations that already set TRONGRID_API_KEY and TRON_RECEIVE_ADDRESS are imported into the database on first startup after this migration; after that, the database settings are authoritative.

The payment settings page exposes the TRON receiving address, optional USDT contract override, enable/disable switch, and API-key replacement. Enabling collection is rejected unless a valid TronGrid key and valid TRON Base58Check address are present.


## Dujiao-Next upstream procurement (PR C draft)

Upstream products are not local card inventory. The local card delivery recovery worker explicitly excludes orders whose snapshotted `delivery_type` is `upstream`. Once a trusted payment path records the order as `paid`, the durable procurement scheduler rechecks that status and the upstream product/SKU mapping before enqueueing a unique local procurement row.

The local order number is sent as Dujiao-Next `downstream_order_no`. The supplier documents idempotency for the same API Key + downstream order number. The official API only documents `GET /orders/:id` by upstream order ID; it does not document a lookup by downstream order number. If a POST times out before the upstream ID is received, the same POST is retried with the same downstream order number, so the supplier returns the existing order under its idempotency contract. The implementation must never retry with a new downstream key.

Supplier order ID and order number are persisted against the local order number. A synchronous fulfillment or a later `GET /orders/:id` result is encrypted at rest before Telegram delivery. Pending supplier orders are polled using the configured interval and deadline. The callback endpoint `POST /api/v1/upstream/callback` validates the documented API Key, timestamp and HMAC-SHA256 signature over the exact raw body; duplicate callback event keys do not create another fulfillment event. Invalid signatures are recorded in a separate security audit table and trigger an Owner notification.

Supplier failures and query timeouts set the local order to `purchase_failed`, preserve `payment_method` and `payment_reference`, do not debit/credit the buyer wallet, and create a durable failed-queue record. Owner manual retry reuses the original local order/idempotency key and resets the configured timeout. Only an explicit Owner refund action credits the buyer's internal wallet; it creates a separate `wallet_transactions` credit entry, updates the order/refund state and writes a procurement audit event. This does not rewrite the original payment record.

The upstream settings page exposes callback URL, poll interval, procurement timeout and supplier-balance warning threshold. Supplier balance is included in the UTC daily reconciliation report and low balance notifies Owner. PR C is currently draft PR #35; no real supplier account integration is claimed. PR D and PR E remain separate, incomplete phases.
