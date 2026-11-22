# LLM Vendor Adapter Compatibility Matrix

Status: implementation contract for P1-2; not a claim of completed adapters.
Baseline: PR #140 AI support foundation branch. Keep the generic OpenAI-compatible transport as the shared HTTP layer; vendor adapters must only add explicit request/response compatibility behavior.

## Supported vendors and verified protocol facts

| Vendor | Chat Completions base URL | Authentication | Verified vendor-specific behavior | Adapter requirement |
|---|---|---|---|---|
| OpenAI | `https://api.openai.com/v1` | Bearer API key | Chat Completions supports `max_completion_tokens`; `max_tokens` is deprecated and unsupported by some reasoning models. Parameter support varies by model. | Select token-limit field by explicit model capability; do not blindly send both. |
| DeepSeek | `https://api.deepseek.com` (API path `/chat/completions`) | Bearer API key | Chat Completions supports `thinking.type` and `reasoning_effort`; thinking defaults on for current documented models. Some sampling/tool parameters are ignored or unsupported in specific modes. | Support explicit thinking toggle/effort only when configured; keep reasoning content separate from user-visible answer; map vendor finish reasons without treating non-stop outcomes as success. |
| Moonshot Kimi | International: `https://api.moonshot.ai/v1`; mainland platform: `https://api.moonshot.cn/v1` | Bearer API key | Official material documents OpenAI-compatible Chat Completions. Endpoint and credentials must match the account platform. | Provide distinct selectable endpoint presets; never infer .ai/.cn from server location. Verify model-specific context/output limits from current model catalog. |
| Zhipu GLM | `https://open.bigmodel.cn/api/paas/v4` | Bearer API key | Official examples use OpenAI-style `chat.completions.create`; responses may include `reasoning_content` for selected model families. Some legacy Assistant APIs are marked deprecated. | Use Chat Completions, not deprecated Assistant/legacy application APIs; parse normal `message.content` safely and ignore reasoning_content for customer replies. |

## Shared adapter contract

1. Preserve `Provider.Complete(context.Context, CompletionRequest)` as the application-facing interface.
2. Keep API keys encrypted at rest; never include keys or provider response bodies in logs/errors.
3. Enforce HTTPS except loopback-only test servers, bounded timeout, bounded response body, and bounded input messages.
4. A provider adapter may add only documented vendor fields. Unsupported fields must be omitted, not guessed.
5. Only assistant `message.content` is eligible for customer-facing output. Never expose reasoning traces, tool calls, or raw provider errors.
6. Treat empty content, malformed JSON, missing choices, non-2xx responses, timeouts, and truncated/oversized bodies as failures so the existing support flow can hand off to a human.
7. No tool execution, refund, fulfillment, balance, price, or order mutation is permitted through the customer-support LLM.
8. Vendor selection and model names are operator-configured. Do not hard-code credentials or assume all models under a vendor support identical parameters.

## Required contract tests

- Endpoint path and bearer authorization for each vendor preset.
- Vendor-specific request JSON fields are present only when enabled and supported.
- Standard Chat Completions response is parsed; reasoning_content is never returned as answer text.
- Empty/malformed choices and non-2xx responses fail closed.
- Provider error body and API key are not leaked.
- Context cancellation and timeout are respected.
- Unknown provider IDs and invalid URLs are rejected.
- Kimi .ai and .cn presets remain independent and are not auto-switched.
- DeepSeek thinking on/off and effort serialization are tested for explicit request configuration.
- OpenAI token-limit field selection is configurable and tested for default/auto modern behavior and explicit legacy `max_tokens`; no model-name capability guessing is performed.

## Source references checked 2026-11-23

- OpenAI Chat Completions API reference: https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create
- DeepSeek Chat Completions API: https://api-docs.deepseek.com/api/create-chat-completion/
- DeepSeek Thinking Mode: https://api-docs.deepseek.com/guides/thinking_mode/
- Moonshot Kimi Model API guide: https://www.kimi.ai/resources/model-api
- Moonshot Kimi provider configuration reference: https://github.com/MoonshotAI/kimi-code/blob/main/docs/en/configuration/providers.md
- Zhipu GLM API examples: https://docs.bigmodel.cn/cn/best-practice/case/ai-search-engine
- Zhipu Assistant API reference (legacy/deprecated; not to be used for this integration): https://docs.bigmodel.cn/api-reference/助手-api/助手对话

## Delivery status (updated 2026-11-23)

Initial code implementation is now present on the PR #141 branch:

- Added a vendor registry for OpenAI, DeepSeek, Moonshot Kimi international (`.ai`) and mainland China (`.cn`), and Zhipu GLM.
- Added provider selection through `NewVendorProvider`; unsupported vendor IDs fail closed.
- Added explicit token-limit serialization: OpenAI uses `max_completion_tokens`; DeepSeek, Kimi and Zhipu use `max_tokens`.
- Routed the Telegram AI support path through the vendor registry.
- Added mock contract tests for vendor token fields, aliases, regional Kimi presets and unknown-vendor rejection.

Still incomplete and not claimed as production-ready:

- DeepSeek thinking-mode enable/disable and reasoning-effort request fields are implemented; exact availability remains model/API-version dependent and must be verified against the selected model.
- OpenAI model capability is not inferred from model names. Operators can explicitly select `TokenLimitParameter` as `auto` (default: `max_completion_tokens`), `max_completion_tokens`, or `max_tokens`; the legacy choice must be selected for models/gateways that require it.
- Vendor-specific response/finish-reason normalization remains partial; non-stop finish reasons now fail closed, and tests verify reasoning_content is not returned as customer-facing text.
- HTTP failures now expose a stable `ProviderError` classification (`auth_failed`, `rate_limited`, `invalid_request`, `upstream_error`, `unexpected_status`) plus a retryable flag. The raw vendor error body is never included in the returned error. `ProviderError` remains compatible with `errors.Is(err, ErrProviderResponse)`.
- Timeout and caller-cancellation contract tests are now present; full malformed/oversized response and per-vendor failure matrix still need execution and expansion.
- Real API integration with operator-provided credentials, CI verification, merge, release and production deployment.

This is a partial implementation, not four fully completed vendor adapters. Existing generic HTTPS, timeout, response-size and secret-redaction safeguards remain in the shared transport.


## OpenAI token-limit configuration

`Config.TokenLimitParameter` is an explicit transport setting for OpenAI endpoints:

- Empty or `auto`: send `max_completion_tokens` (default).
- `max_completion_tokens`: send the modern field explicitly.
- `max_tokens`: send the legacy field for a model or compatible gateway that requires it.
- Any other value fails with `ErrInvalidConfig`.

The transport intentionally does not infer support from model-name prefixes. The operator must select the parameter appropriate to the configured model and endpoint. This field is currently a provider configuration capability; the Telegram admin UI wiring remains a separate follow-up.
