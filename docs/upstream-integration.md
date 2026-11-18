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

PR A（PR #21）已合并至 main，新增 Dujiao-Next 配置存储、双凭证加密、连接测试及状态展示。

PR A 的边界仅限配置与连接测试。PR B（开发中）实现分类/商品/SKU 同步、上游图片本地化、目录来源切换、缺失商品隐藏及同步状态管理；同步失败不覆盖上次成功目录，配置/凭证/价格策略变更会将目录标记 stale 并要求重新同步后才能启用。

价格处理：Dujiao ping 返回站点币种。TG-Card 订单结算使用 USDT，因此上游币种为 USDT 时按 1:1；其他币种必须在后台手动配置“1 单位上游币种 = ? USDT”的汇率。最终售价 = 上游 SKU 原币价格 × 汇率 ×（1 + 百分比加价）+ 固定 USDT 加价。原币金额与币种作为商品快照保存；不自动抓取或猜测汇率。

PR B 不执行上游采购、付款转发、查单/回调、失败队列或退款。为避免已付款但无法履约，在 PR C 采购链路完成前，Bot 可浏览上游目录但不显示购买确认入口，服务端也拒绝上游商品订单创建。上述能力按 PR C、D、E 继续独立实施。

Dujiao-Next API Key 与 API Secret 均使用 TG-Card 的持久化 AES-256-GCM 密钥加密；读取接口只返回掩码/已配置标志。连接测试按官方 HMAC-SHA256 规则调用 `POST /api/v1/upstream/ping`。异次元 ACG-Faka 与萌次元 MCY 在取得可核验商户协议前不实现任何 adapter。生产真实商户验收由项目所有者在测试环境执行。

