## Independent payment providers (implemented; live merchant acceptance tracked separately)

- Epusdt and GMPay Edge are configured as separate merchant endpoints with independent PID, encrypted Secret, HTTPS notify URL, and enable flags. Do not reuse credentials across providers.
- A local payment intent stores an immutable `provider` value (`epusdt` or `gmpay_edge`). Reopening checkout must resume through that recorded provider; a later settings change must not migrate the existing order.
- Epusdt status checks use its public `/pay/check-status/{trade_id}` endpoint. GMPay Edge status checks use the authenticated, signed `GET /payments/gmpay/v1/order/query` endpoint with exactly one of `trade_id` or `order_id`.
- GMPay Edge callbacks are received at `/api/payment/gmpay/notify`, authenticated with the GMPay Edge PID/Secret, and acknowledged with plain-text `ok` only after local processing succeeds.
- Implemented and released in PR #121/#122: Telegram checkout rotates among enabled TRC20, Epusdt, and GMPay Edge methods. The selected provider is stored on the order intent; TRC20 chain scanning rejects a transfer when the order intent belongs to a different provider. Checkout creation, callback signature verification, status query, atomic order payment confirmation, idempotent settlement, and delivery handoff are implemented. Real merchant collection, public callback reachability, and end-to-end delivery still require acceptance in the target deployment environment.
## P0-4 补充 — 不确定交付的手动重试边界

- 普通单步骤交付（卡密、固定文本、上游内容等）进入 `delivery_uncertain` 后，后台不得直接重新发送；必须先人工核实 Telegram 是否已送达，避免同一权益/消息重复发送。
- `file_card` 是唯一允许在人工核实步骤后恢复的多步骤类型：必须存在分步骤记录，且每个步骤均已明确为 `succeeded` 或 `failed`；存在 `uncertain`、`sending` 或缺失步骤证据时，数据库 claim 层拒绝重试。
- 自动恢复扫描继续排除 `delivery_uncertain`；本次同时在数据库 claim 与管理员 HTTP 入口加固，避免绕过前端限制直接领取不确定订单。
- 新增 SQLite/PostgreSQL 回归测试覆盖普通单步骤不确定订单拒绝手动 claim，以及 file_card 缺失/未核实步骤拒绝、全部核实后允许恢复失败步骤。
- 无数据库迁移、无新增配置；不改变明确失败 `delivery_failed` 的既有重试行为。

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

After `ConfirmOrderPayment` succeeds, the immediate payment path and admin manual-retry endpoint first acquire the order's two-minute delivery lease with `database.ClaimOrderDelivery`. The background worker uses `database.ClaimOrderDeliveries` for batches. Only the lease owner loads the paid card with `database.GetOrderDelivery` and calls `telegrambot.Manager.DeliverPaidOrder`. This prevents the payment watcher, background worker, and admin retry action from dispatching the same order concurrently. The manager decrypts the stored card key and sends it to the order's Telegram chat. It never writes plaintext card keys to logs.

If the process stops after payment confirmation, `database.GetOrderDelivery` can recover the paid card for orders in `paid` or `delivery_failed`. The authenticated admin endpoint `POST /api/orders/{id}/retry-delivery` invokes the same delivery path and records an audit event.

A background recovery loop also scans for due deliveries every 15 seconds. It claims up to 50 orders in a database transaction and sets a two-minute `delivery_lease_until` lease. PostgreSQL workers use `FOR UPDATE SKIP LOCKED`; SQLite uses its serialized write transaction. A worker restart leaves the order in `delivering`, and the next worker can reclaim it after the lease expires. Successful delivery clears the lease; failed delivery records the error and returns the order to `delivery_failed` for another scan. This lease prevents concurrent workers from normally sending the same order at the same time, but it does not remove Telegram's at-least-once delivery limitation described below.\n\nAutomatic retries for `delivery_failed` orders use exponential backoff based on `delivery_attempts`: 15, 30, 60, 120, 240, and 480 seconds, then a 900-second cap. Newly paid orders are still eligible immediately, and expired `delivering` leases remain recoverable. The authenticated admin manual-retry endpoint uses the single-order lease path and intentionally bypasses this background backoff, so an operator can retry immediately after reviewing the failure.

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
- GMPay protocol client (PR #116) and Epusdt merchant configuration (PR #117) are merged. Telegram checkout, callback verification, order settlement and delivery handoff are implemented in PR #121 and remain subject to PR checks and release.

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


## Dujiao-Next procurement state

The independent Dujiao-Next procurement implementation was merged in PR #32. Paid upstream orders are registered in the durable procurement queue, rechecked before supplier submission, and tracked by local order number plus supplier order ID/number. Synchronous fulfillment, polling, and signed callbacks converge on the same encrypted fulfillment/delivery path.

If the initial supplier POST times out before returning an upstream order ID, the order is parked as `uncertain` for Owner verification; it is not automatically resubmitted. The documented supplier API supports idempotency by API Key + `downstream_order_no`, but does not document a GET lookup by that local key. Manual recovery reuses the original key. Known supplier order IDs are queried using `GET /orders/:id`.

Supplier failures/timeouts do not alter the original buyer payment record or automatically debit/refund the buyer wallet. Owner-confirmed refunds atomically commit the separate wallet credit, refund ledger, order state, procurement event, and admin audit in one database transaction. Dujiao-Next normal integration has been confirmed complete by the project owner; no additional PR E merchant-acceptance gate is required.

## PR D — Supplier funds and refund safety (PR #38 merged)

- The PR2 wallet reconciliation remains read-only and compares each buyer's stored balance with credit-minus-debit ledger entries.
- PR D adds supplier balance snapshots immediately before and after a procurement request, linked to the local order number, supplier order ID, and supplier-reported amount. A mismatch or unavailable snapshot creates a finance exception for manual review; it never changes supplier or buyer balances.
- The first pre-purchase baseline is retained across idempotent retries. Supplier-side concurrent transactions may affect the observed balance delta, so the result is an audit signal, not an automatic accounting adjustment.
- A periodic sweep marks unresolved post-purchase snapshots as unverifiable after 15 minutes.
- Refund integrity checks validate the refunded order, independent refund row, admin/reason, buyer, amount, and the unique wallet credit transaction. Missing/orphan links create exceptions; the checker does not repair ledger entries.
- Uncertain orders have an ageing view and an Owner resolution workflow. Confirmed accepted orders require a positive supplier order ID, extend the polling deadline, and resume GET-only polling; confirmed not-accepted orders are moved to failed state for explicit retry using the original idempotency key.
- A unified finance exception center covers procurement failures, uncertain orders, supplier balance differences, and refund ledger anomalies. PR #38 was merged in commit `935bd222977b1a636c5f05ab4faebdc3e835dda1`; Go CI #44370639264, Admin CI #44370639142, Security #44370639404 passed, and GHCR digest `sha256:691493ce844c96a504785e0fb72b24534786f57279190322332f45b5ba8b7b29` was published. Production deployment and acceptance remain separate; PR E remains the only live supplier-money integration phase.

## Telegram delivery outcome uncertainty

Telegram Bot API sends do not provide a business idempotency key for order delivery. If a sendMessage/sendDocument request loses its connection or its response cannot be read, the application cannot prove whether Telegram accepted the message. Retrying such a request automatically can deliver the same card key, file, or upstream fulfillment payload more than once.

Before the first Telegram delivery side effect, the order is durably moved to `delivery_uncertain`. This also covers a process crash after the send but before the final delivered-state write. The background delivery worker does not automatically claim this state. An administrator must inspect the order and, where possible, confirm with the customer whether the entitlement arrived before explicitly choosing a manual retry. The order page displays a warning and asks for confirmation before the manual retry path is invoked.

Failures detected before any Telegram send is attempted (for example, invalid local delivery configuration or a decryption failure) remain `delivery_failed` and use the normal exponential retry schedule. This is a conservative at-most-one-automatic-attempt policy for ambiguous remote outcomes, not an exactly-once delivery guarantee. The operator may still choose a manual resend, which can result in duplicate delivery if Telegram had accepted the original request. One-time invite links have an additional restriction: if link creation or delivery is in delivery_uncertain, the application blocks the normal retry path. Telegram may already have created an active invite link even if the response or subsequent local persistence failed; retrying could create another live link. Such orders require manual verification and handling rather than another automatic link creation. The same whole-order retry restriction applies to `file_card`: it sends a document and then a separate card-key message. If the document was accepted but the card-key message has an ambiguous outcome, retrying the whole order can duplicate the document. The admin API and UI therefore require manual verification for uncertain `file_card` orders; this is a conservative quarantine, not stage-level automatic resume.


## Customer re-delivery safety

Customer and administrator re-delivery of an already delivered `file_card` order is not safe to automate: the flow sends a document and a separate card-key message. If the first step succeeds and the second fails, the order remains delivered; retrying the whole pair can send the document again. PR #72 blocks this re-delivery path in the Telegram user interface, Manager, and admin API, and directs the customer/operator to manual verification. This is a conservative whole-order restriction, not stage-level resume. Single-step delivery types retain their existing re-delivery behavior; one-time invite links remain non-reissuable.


## PostgreSQL re-delivery lease concurrency

Customer/admin re-delivery is protected by a per-order database lease. The PostgreSQL implementation uses an atomic upsert against the unique `order_redelivery_leases.order_id` key; only one concurrent worker should acquire an active lease, and an expired lease may be reclaimed. PR #73 adds PostgreSQL 16 integration coverage for concurrent claimants, active-lease exclusion, and expired-lease recovery. The test does not change the runtime algorithm or schema.


## Refund and customer re-delivery mutual exclusion

Refunding an already delivered order must not race with a customer/admin re-delivery request. Both operations serialize on the order row: claiming a re-delivery lease locks the delivered order before writing the lease, and the wallet-refund transaction checks for an unexpired re-delivery lease while holding that same row lock. Refund is rejected while a re-delivery lease is active; after the lease expires, refund may proceed. Once refunded, the order cannot acquire another re-delivery lease. This uses the existing `order_redelivery_leases` table and adds no schema migration.


## Auditing refund attempts blocked by re-delivery

When a refund is blocked because an active re-delivery lease exists, the support refund API returns HTTP 409 and records a `support.refund.blocked` admin audit event with the order and reason. Other refund failures are also recorded as `support.refund.failed`. The database exposes a typed conflict error so the API does not classify this condition by matching error-message text.


## 全局异常中心联动

当 Telegram 交付进入 `delivery_uncertain` 时，同一数据库事务创建或刷新 `system_exceptions` 记录，来源键为 `delivery-uncertain:<order-id>`。订单与交付事件仍是事实来源；全局异常中心仅提供聚合视图。标记解决不会重发 Telegram 消息、改变订单状态，也不能证明远端已送达。必须先核实真实 Telegram 结果，再记录处理结论。详见 `docs/system-exceptions.md`。

## File-card step-level delivery state machine (P0-3)

`file_card` consists of two independent Telegram side effects: `file` (sendDocument) and `card_message` (sendMessage). Each step is persisted in `order_delivery_steps` with its own attempt count, status, last error, timestamps, and Telegram `message_id` when the API returns one.

- A definitive Telegram rejection marks only that step `failed`; an explicit admin retry skips every `succeeded` step and retries only `failed` steps.
- A transport timeout, malformed response, HTTP 5xx, missing `message_id`, or process crash while a step is `sending` is treated as `uncertain`. Such a step is never resent until an administrator verifies whether Telegram accepted it and records `succeeded` or `failed` with an audit note.
- The order delivery lease remains active while an external request is in flight. A second administrator cannot claim or resolve a step until the current request completes or the lease expires; after an expired lease, a leftover `sending` step is converted to `uncertain` before any retry.
- A step manually marked `succeeded` is skipped on retry; a step manually marked `failed` becomes eligible for the next explicit retry. Resolution and audit are committed in one transaction.
- Migration 0053 backfills pre-existing `delivery_uncertain` `file_card` orders as two `uncertain` steps; an operator must verify the file and card-message outcomes separately before either can be retried.
- The order can become `delivered` only after all persisted steps are `succeeded`. Re-delivery of an already delivered `file_card` remains blocked; this state machine is for completing the original paid delivery, not duplicating a delivered entitlement.
- This improves stage-level recovery but does not provide Telegram exactly-once semantics. A step with an ambiguous remote outcome still requires human verification.


## 退款与首次交付并发边界

- `RefundOrderToWallet` 与首次交付领取使用同一订单行作为并发仲裁点。
- 订单处于 `delivering` 时，代表首次交付请求正在执行或其结果尚未完成恢复核实；退款必须拒绝，不能仅因为钱包退款事务能够取得订单锁就继续入账。
- 只有订单明确处于 `paid`、`delivery_failed` 或 `delivered` 等允许人工退款的状态时，才继续执行现有退款流程；`delivery_uncertain`、`delivering`、已退款及其他非法状态均拒绝。
- 拒绝退款不修改订单、卡密、钱包余额或资金流水。订单必须先由交付恢复/人工核实流程收敛到明确状态。


## Epusdt GMPay merchant configuration (PR #116/#117 merged)

The GMPay protocol client (PR #116) and Epusdt merchant configuration (PR #117) are merged. Merchant credentials are managed in the existing administrator payment settings. The gateway base URL and PID are stored as ordinary configuration; the merchant Secret is encrypted with the application's persistent AES-256-GCM secret key. The read API returns only a boolean indicating whether a Secret is configured. Leaving the Secret field blank preserves the current value; an explicit clear action removes it.

Historical phase boundary (PR #117): migration 0058 originally delivered merchant configuration only; it did not itself provide checkout or settlement. That limitation was subsequently closed by PR #121/#122, which implemented and released the complete checkout/payment lifecycle described below. Configuration alone must still not be treated as proof of successful live merchant collection.

## Epusdt GMPay checkout lifecycle (PR #121/#122 merged and released)

The existing GMPay client and merchant configuration are now wired into the Telegram storefront.

- The product purchase flow and unified cart checkout expose a hosted GMPay cashier option when enabled.
- The merchant order number is the Epusdt `order_id`; the provider requires it to be unique and at most 32 characters. TG-Card creates one local payment intent per local order and never blindly retries a provider create after an ambiguous timeout.
- Requests use `POST /payments/gmpay/v1/order/create-transaction`, `currency=usd`, `token=usdt`, and `network=tron`. The local USDT-denominated amount is sent as a two-decimal USD amount.
- The public callback endpoint is `POST /api/payment/epusdt/notify`. It requires the configured HTTPS notify URL, validates PID and HMAC-SHA256 signature, checks order/trade binding and exact minor-unit amount, then uses the existing order payment guard before delivery.
- The user can request a status refresh; TG-Card queries the provider using `GET /pay/check-status/{trade_id}`. A confirmed status is bound to the exact local trade ID and amount before settlement.
- Duplicate callbacks are idempotent. Amount or token mismatches, unexpected/duplicate trade IDs, and payments arriving after local order cancellation are recorded as payment events and do not trigger a second delivery.
- An ambiguous create timeout is recorded as `create_uncertain`; because Epusdt merchant `order_id` values cannot be reused and the public status API requires a provider `trade_id`, TG-Card does not issue a second create request. A later signed callback can reconcile the original local order; otherwise the Owner must inspect the provider merchant console.
- Historical pre-merge note: PR #121 的 CI/合并/镜像发布在该条记录形成时尚未完成；后续 PR #121 已合并并发布，PR #122 进一步完成三通道整合。当前实现状态以本文顶部“implemented”说明及 PROJECT_STATUS.md 顶部记录为准。


## Late TRC20 payment reconciliation audit requirement

Manual reconciliation of an order whose status is expired, or whose expiry timestamp has passed, requires a non-empty operator note describing the verification basis. The check is enforced inside the same database transaction that locks the transfer and order; a missing note leaves the transfer unmatched and does not mutate the order or stock. This is an audit control only: it does not automatically refund, credit a wallet, or re-open an order without an explicit, successful reconciliation.
