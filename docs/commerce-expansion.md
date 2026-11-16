# Commerce expansion — implementation contract

## Product availability notifications
- Buyers can opt in on an out-of-stock product. Subscription is idempotent per product + Bot + Telegram user.
- When stock is replenished, the originating Bot sends the buyer a product/price notice with an open-product action. A notification is marked sent only after Telegram confirms delivery; failures remain retryable.
- The notification never creates an order or reserves inventory.

## Quantity discounts
- Product-level optional tiers use minimum quantity and percentage/fixed unit-price discounts.
- Checkout recalculates the effective price server-side and reserves all requested stock in one transaction. If stock is insufficient, no partial reservation or order is created.
- Historical order snapshots preserve unit price, discount and final total. No tiers means today's unchanged single-unit price.

## Recharge rewards
- A global admin toggle and configurable threshold tiers support fixed or percentage bonuses.
- Disabled/empty rules mean normal recharge with no bonus. One best matching tier applies, not cumulative tiers.
- Bonus is snapshotted when the recharge is created and credited exactly once with the principal on confirmed payment; old pending recharge records are not retroactively changed.

## Focused Telegram menus
- “My balance” displays Telegram user ID and current balance with only Recharge and Refresh actions.
- Recharge shows only configured amount choices, custom amount and Back.
- Orders and support screens show only context-specific actions and a Back action; never attach the full main menu to every response.
- The configured Bot owner has a separate owner menu and only owner-authorized product-management actions.

## Delivery modes
- card: encrypted stock key(s), quantity-aware.
- fixed_text: configured text sent after payment.
- invite_link: create a Telegram invite link after payment with member_limit=1 and expiry; never reuse an entitlement.
- file: send the configured Telegram document `file_id`.
- file_card: send configured file together with reserved card key(s).
- The Bot owner can send a document to the Bot in a private chat; the Bot replies with its Telegram `file_id` for the product editor. The file must be sent to the same Bot that will fulfill the order.
- Card keys are atomically reserved and consumed by order. Invite links are created with `member_limit=1` and an expiry, then encrypted and persisted before delivery so retries reuse the same link. Paid entitlements are never released for a second buyer after a Telegram send failure.

## Admin UI and acceptance
- Product editor includes optional discount tiers, delivery mode selector and mode-specific settings; the Telegram Bot owner can retrieve a document `file_id` by sending the file to the Bot.
- Marketing settings include recharge bonus enable/disable and threshold tiers.
- Product stock view shows notification subscribers.
- All settings are database-backed; no new environment variables or manual SQL.
- Acceptance requires automated tests for discount arithmetic, atomic multi-key reservation, reward idempotency, subscription deduplication and single-claim delivery assets, plus admin UI build and Telegram end-to-end tests for each delivery mode.
