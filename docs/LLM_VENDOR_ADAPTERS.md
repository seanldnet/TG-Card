# LLM Vendor Adapter Compatibility Matrix

Status: first adapter batch merged in PR #142; remaining gaps and live integration are tracked below.
Baseline: PR #140 AI support foundation and PR #142 first adapter batch are merged to main (merge commit `44fb370850c97c0c85dc1bddd2326fd38ae28a55`). Keep the generic OpenAI-compatible transport as the shared HTTP layer; vendor adapters must only add explicit request/response compatibility behavior.

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
- Context cancellation and timeout are respected and classified separately: cancellation is non-retryable, while transport deadlines/timeouts are retryable.
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

## Delivery status (updated 2026-11-23; PR #142 merged)

PR #142 is merged to main. The first adapter batch includes:

- Vendor registry and provider selection for OpenAI, DeepSeek, Moonshot Kimi international (.ai) and mainland China (.cn), and Zhipu GLM.
- Explicit token-limit serialization: OpenAI uses the configured `max_completion_tokens` / legacy `max_tokens` choice; DeepSeek, Kimi and Zhipu use `max_tokens`.
- Telegram AI support routed through the vendor registry.
- Mock contract tests for token fields, aliases, regional Kimi presets, unknown vendors, timeout/cancellation, malformed/empty responses, oversized bodies and per-vendor HTTP failure classification.
- Stable provider error classifications and fail-closed handling for non-stop finish reasons; reasoning content is not exposed as customer-facing text.

Remaining gaps (not production-ready claims):

- DeepSeek thinking enable/disable and reasoning-effort are not exposed as operator controls in the Telegram support path/admin settings.
- Vendor-specific response and finish-reason normalization is partial.
- No live API integration has been verified with operator-provided credentials.
- Production deployment and end-to-end production verification are separate operational steps.

PR #142 merge commit: `44fb370850c97c0c85dc1bddd2326fd38ae28a55`.

## OpenAI token-limit configuration

`Config.TokenLimitParameter` is an explicit transport setting for OpenAI endpoints:

- Empty or `auto`: send `max_completion_tokens` (default).
- `max_completion_tokens`: send the modern field explicitly.
- `max_tokens`: send the legacy field for a model or compatible gateway that requires it.
- Any other value fails with `ErrInvalidConfig`.

The transport intentionally does not infer support from model-name prefixes. The operator must select the parameter appropriate to the configured model and endpoint. This setting is persisted with the AI support configuration and exposed in the administrator's AI Support settings. Existing saved configurations without this field default to `auto`; the API response exposes only the selected parameter and never the API key. The custom OpenAI-compatible gateway option remains supported and requires an explicit HTTPS BaseURL.


## Endpoint preset persistence

For a built-in vendor, an omitted BaseURL is resolved to that vendor's documented default before the configuration is persisted. The custom OpenAI-compatible gateway does not have a default endpoint and must be given an explicit BaseURL. The saved configuration therefore always contains the effective endpoint rather than an empty preset value.
