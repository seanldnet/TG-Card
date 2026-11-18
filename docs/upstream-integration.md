# 上游发卡系统对接调研（协议证据核验）

> 核对日期：2026-11-18。本文区分官方文档、上游源码和待确认信息；协议存在不代表 TG-Card 已实现对接。

## 结论矩阵

| 上游 | 分类/商品 | 商户下单 | 交付 | 余额 | 证据结论 |
|---|---|---|---|---|---|
| Dujiao-Next | 官方 Open API：GET /categories、GET /products，分页和 SKU 结构有定义 | POST /orders，HMAC-SHA256；downstream_order_no 支持下游幂等关联 | GET /orders/:id + POST /upstream/callback；履约数据在 delivered 后提供 | POST /ping 返回 balance/currency | 官方文档确认，尚未在 TG-Card 实现 |
| 异次元 / ACG-Faka | 未确认官方商户目录 API | 未确认官方商户采购 API | 未确认 | 未确认 | 待上游确认；第三方支付插件不构成货源 API 证据 |
| 萌次元 / MCY Shop | 官方 Wiki 描述货源插件体系，但未确认第三方商户目录 API | 未确认 | 未确认 | 未确认 | 待上游确认；不得从异次元或 Dujiao 协议推断 |

## 1. Dujiao-Next（官方文档确认）

证据：
- https://dujiao-next.com/api/integration-open-api
- https://dujiao-next.com/api/integration-guide

API 根路径为 `/api/v1/upstream`。认证头为 `Dujiao-Next-Api-Key`、`Dujiao-Next-Timestamp`、`Dujiao-Next-Signature`。签名算法 HMAC-SHA256，原文 `{METHOD}\n{PATH}\n{TIMESTAMP}\n{BODY_MD5}`，API Secret 为密钥，时间戳容差 ±60 秒。

| 用途 | 官方接口 | 文档确认的关键内容 |
|---|---|---|
| 连接/余额 | POST /ping | 返回 ok、site_name、user_id、balance、currency、member_level |
| 分类 | GET /categories | categories[]：id、parent_id、slug、name（多语言）、icon、sort_order |
| 商品 | GET /products | page、page_size；items[]、total、page、page_size；商品包含 id、slug、title、description、content、images、tags；下单使用 SKU ID 而非商品 ID |
| 下单 | POST /orders | sku_id、quantity、可选 manual_form_data、downstream_order_no；上游钱包自动扣款 |
| 查单 | GET /orders/:id | 查询采购单；已交付时提供 fulfillment 信息 |
| 取消 | POST /orders/:id/cancel | 上游取消接口 |
| 异步回调 | POST /upstream/callback | 上游推送状态与交付内容 |

官方站点对接指南明确支付后异步采购、回调传递状态和交付内容；采购失败时标记异常供管理员处理，默认不自动取消/退款。TG-Card 需自行实现持久化队列、失败人工处理、重试与退款审计。回调负载的完整字段和签名校验规则应以 Open API 对应章节为准，未完成联调前不写入假设。

## 2. 异次元 / ACG-Faka（待上游确认）

本次未取得可核验的官方商户采购 API 文档或源码证据。公开第三方资料中可见支付插件、支付回调和支付通道适配示例，但它们不能证明存在商品目录同步、商户余额查询、采购下单及卡密交付 API。

待上游提供：目录/商品字段与分页、商户认证/签名、余额接口、下单字段与幂等键、同步交付或异步查单/回调、下架/失败/撤单语义。全部未确认前不实现 adapter。

## 3. 萌次元 / MCY Shop（待上游确认）

官方 Wiki：https://wiki.mcy.im/；官方源码仓库：https://github.com/lizhipay/mcy-shop 。公开资料描述插件体系和货源插件能力，但本次未能核实面向第三方商户的统一目录/采购 API。Wiki 页面抓取受限，不能据此补写字段。

待上游确认：是否提供外部商户 API、目录/图片/价格字段、商户 ID/密钥与签名、余额查询、采购下单与幂等、同步卡密或异步查单/回调、失败和退款语义。不能因为产品历史渊源推断与异次元协议兼容。

## 4. TG-Card 实施边界

PR A（PR #21）已合并至 main，新增 Dujiao-Next 配置存储、双凭证加密、连接测试及状态展示。PR B（PR #26）已合并至 main，目录同步、图片本地化、价格策略与来源切换已实现；真实商户连接/同步验收仍待项目所有者执行。

PR A 的边界仅限配置与连接测试。PR B（PR #26，已合并）已实现分类/商品/SKU 同步、上游图片本地化、目录来源切换、缺失商品隐藏及同步状态管理；同步失败不覆盖上次成功目录，配置/凭证/价格策略变更会将目录标记 stale 并要求重新同步后才能启用。

价格处理：Dujiao ping 返回站点币种。TG-Card 订单结算使用 USDT，因此上游币种为 USDT 时按 1:1；其他币种必须在后台手动配置“1 单位上游币种 = ? USDT”的汇率。最终售价 = 上游 SKU 原币价格 × 汇率 ×（1 + 百分比加价）+ 固定 USDT 加价。原币金额与币种作为商品快照保存；不自动抓取或猜测汇率。

PR C 已在独立 PR #32（Draft）实现采购转发、订单状态复核、幂等、异步查单/回调、人工失败队列、退款账本和供应商余额日报；Go/Admin/Security CI 与十项 mock 测试均通过，但 PR 尚未合并，不代表生产可用。PR D 与 PR E 仍未完成。

Dujiao-Next API Key 与 API Secret 均使用 TG-Card 的持久化 AES-256-GCM 密钥加密；读取接口只返回掩码/已配置标志。连接测试按官方 HMAC-SHA256 规则调用 `POST /api/v1/upstream/ping`。

### PR A~E 实施状态与严格边界

| 批次 | 范围 | 当前状态 |
|---|---|---|
| PR A（PR #21） | 上游基础设施、商户配置、双凭证加密、连接测试 | 已合并 |
| PR B（PR #26） | 分类/商品/SKU 目录同步、图片本地化、加价展示、目录来源切换 | 已合并；真实商户验收待执行 |
| PR C（PR #32） | 支付确认后采购、`downstream_order_no` 幂等、查单轮询/回调、失败人工队列、Owner 通知、手动恢复、站内余额退款账本、供应商余额告警 | **独立 Draft PR；Go/Admin/Security CI 和十项 mock 测试通过，尚未合并** |
| PR D | 供应商资金与本地资金安全控制、余额/扣款一致性核对、异常告警与审计保护 | **待开发** |
| PR E | 使用真实 Dujiao-Next 商户账号进行连接、目录、采购、扣款、异步回调、交付、失败与退款全链路联调 | **待开发/待验收** |

PR C 资金红线：以 TG-Card 订单 ID 作为下游幂等标识；重复请求不得重复供应商扣款。采购失败不得扣减或吞掉买家资金；失败订单进入持久化人工处理队列并通知 Owner。只有确认供应商未履约/已退款等可核实状态后，才按明确业务规则将退款记入买家站内余额，并写入完整审计；不得静默自动改账或在不确定上游状态时重复退款。供应商扣款结果、异步回调和主动查单必须进行状态幂等与冲突检测。

PR C、PR D、PR E 均独立 PR，按 C→D→E 顺序推进；采购链路不得与 PR2 资金/库存安全批次混合。PR E 未完成真实商户端到端验收前，不得宣传上游采购闭环已可用于生产。

异次元 ACG-Faka 与萌次元 MCY：在取得并核验官方商户 API/协议证据前，**不开发适配器、不猜测协议、不复用 Dujiao-Next adapter**。第三方支付插件或货源插件介绍不构成采购 API 证据。此限制持续有效。


## 5. PR C/D/E 实施状态（2026-11-18）

- PR C（Dujiao-Next 采购转发）已在独立 Draft PR #32 实现；Go/Admin/Security CI 与十项 mock 测试通过，尚未合并。合并前仍不得标记为生产已实现或已联调。
- PR C 当前实现：支付确认后复核本地订单/商品状态；以本地订单号作为 `downstream_order_no`；持久化本地/上游订单双向映射；取得上游订单 ID 后按配置轮询；回调验签、去重、加密交付；失败人工队列、手动恢复、独立退款账本/审计；供应商余额日报和阈值告警。
- 官方 API 未提供按 `downstream_order_no` 查询接口。首次 POST 网络异常且未取得上游订单 ID 时进入 `uncertain` 并暂停自动重发；人工恢复沿用原幂等键。不得盲目换幂等号造成重复扣款。
- 回调端点 `POST /api/v1/upstream/callback`，按官方 HMAC-SHA256 原始请求体签名、API Key 与 ±60 秒时间戳校验。后台必须配置公网 HTTPS 回调地址后才允许上游商品自助下单。
- 当前仅开放 `auto` 自动交付 SKU；`manual` 表单 SKU 暂不开放自助购买，待买家表单采集能力实现。
- 十项 mock 测试包括同步交付、异步查单成功、查单超时、采购失败、重复回调、验签失败、超时后同幂等键恢复、已退款订单跳过、HTTP 402/404 错误分类；均在 Go CI 通过。mock 不代表真实上游联调。
- PR D（进一步资金安全控制）与 PR E（真实上游联调）仍未完成。PR E 才执行真实商户凭证、供应商扣款、回调公网可达与交付全链路验收。
