# TG-Card 上游发卡系统对接调研与设计

> 调研基线：2026-11-18。本文区分官方文档、上游开源源码和未核实信息。接口可得性标注为“文档确认”“源码确认”或“待上游确认”；未确认内容不得进入可调用实现。

## 1. 产品与安全边界

TG-Card 仍是单商户、单管理员、多 Telegram Bot 商城。上游对接是单商户采购来源扩展，不引入多租户、代理商、注册或额外用户角色。

- 买家只与 TG-Card 交互，使用 TG-Card 自己的 TRC20/站内余额支付。
- 上游凭证只由管理员维护，商户密钥必须使用项目现有 AES-256-GCM 持久化加密机制；明文不得写数据库、日志或 API 响应。
- 上游下单只能在 TG-Card 确认付款后触发；同一 TG-Card 订单必须具备持久化幂等约束。
- 上游失败不能撤销或吞掉买家已付款事实；订单必须保留资金、上游请求和失败原因以供人工处理。
- 上游未接入时维持本地商品行为。上游启用并成功同步时，展示策略由统一 catalog service 决定；本地商品仅隐藏、不删除。启用状态切换不得误改本地商品 status。
- 当前仅完成协议调研与适配器契约设计；尚无生产可用上游连接、目录同步、采购转发或真实交易验收。

## 2. 上游协议核实矩阵

| 能力 | 异次元发卡（ACG-Faka） | 萌次元（MCY Shop） | Dujiao-Next |
|---|---|---|---|
| 官方资料 | `https://faka.wiki/zh-cn/dev/api.html` | `https://wiki.mcy.im`（当前抓取 403）；官方仓库 README 指向该 Wiki | `https://dujiao-next.com/api/integration-open-api` |
| 开源源码 | `lizhipay/acg-faka` | `lizhipay/mcy-shop` | `dujiao-next/dujiao-next` |
| 商户鉴权 | 文档确认：POST 参数 `app_id` + `sign`；`app_key` 本地参与签名 | 待上游确认：源码看到 User/Merchant/PostDecrypt 会话拦截器及货源 API Code，不等同于文档化的站点间商户密钥 API | 文档确认：API Key + API Secret；HMAC-SHA256 请求头 |
| 分类/商品目录 | 文档+源码确认：`POST /shared/commodity/items` 返回分类数组及 `children` 商品；列表无分页参数 | 源码确认：用户货源 API 的分页列表、分类、SKU；但认证是 MCY 用户/商户会话机制，能否作为外部商户 API 使用待确认 | 文档确认：`GET /categories`、`GET /products?page&page_size` |
| 图片/介绍 | 源码确认：商品白名单含 `cover`、`description`；具体 URL 是否稳定可下载待实测 | 源码确认：货源条目有 `picture_thumb_url`、SKU 有图片字段；外部访问方式待确认 | 文档确认：商品 `images[]` URL、`description`、`content` |
| 价格/库存 | 文档确认：`price`、`user_price`、`stock`；详情/库存接口提供 `factory_price`、库存和 SKU 配置 | 源码确认：SKU 的 `stock_price`、`supply_price`、库存读取/货源采购逻辑；对外协议字段待确认 | 文档确认：`price_amount`、SKU `price_amount`、`stock_status`、`stock_quantity` |
| 上下架 | 源码确认：列表筛选分类 `status=1`、商品 `api_status=1` 且 `status=1` | 源码确认：货源列表筛选 `status=2`、`is_review=0`；是否为外部供货 API 状态字段待确认 | 文档确认：列表仅返回已上架商品；商品和 SKU 有 `is_active` |
| 下单 | 文档确认：`POST /shared/commodity/trade`；必填 `shared_code`、`num`，返回 `tradeNo`、`secret` 等 | 待上游确认：源码存在会话态货源采购 `/user/api/shop/supply/trade`，不是已核实的独立商户签名接口 | 文档确认：`POST /orders`，`sku_id`、`quantity`、`downstream_order_no`；上游钱包自动扣款 |
| 查单/交付 | 文档确认：`POST /shared/commodity/query`，`tradeNo` 查询 `secret`、`status`；手动发货的异步状态细节待实测 | 待上游确认：源码有货源订单/交付处理，但没有核实可供外部站点调用的订单查询 API | 文档确认：`GET /orders/:id`；交付完成后 `fulfillment.payload` / `delivery_data`；另有 callback |
| 余额查询 | 待上游确认：当前公开 API 文档未列余额端点；不能推断存在 | 待上游确认 | 文档确认：`POST /ping` 返回对接钱包 `balance`、`currency` |
| 当前可实现程度 | 目录、下单、查单协议可按文档实现；图片和交易细节需测试账号验收 | 不实现协议适配器，等待官方可公开调用的货源/商户 API 规范 | 目录、下单、查单、余额协议可按文档实现；需上游账号做真实验收 |

## 3. 异次元发卡（ACG-Faka）协议

### 3.1 鉴权

- 请求方式：POST；推荐 `application/x-www-form-urlencoded`。
- 公共参数：`app_id`、`sign`。
- 商户密钥字段：`app_key`，仅用于本地签名，不应上传。
- 签名规则：移除 sign；按参数名排序；移除值为空字符串的字段；计算 `MD5(urldecode(http_build_query(sorted_params) + "&key=" + app_key))`。
- 参数包含数组（如 `sku`）时，数组内容也参与签名，签名序列化必须与实际请求完全一致。
- 响应统一为 `code`、`msg`、`data`；`code=200` 表示成功，失败通常为 `code=0`。

### 3.2 目录和商品

- `POST /shared/commodity/items`：返回分类数组；分类包含 `id`、`name` 等字段，`children` 是商品列表。商品包含 `id`、`name`、`code`、`price`、`user_price`、`stock`、`delivery_way`、`draft_status` 等。
- 该接口没有文档化分页参数；不可自行添加 page/limit。
- `POST /shared/commodity/item`：使用 `code` 查询详情。文档列出 `name`、`description`、`code`、`price`、`user_price`、`stock`、`delivery_way`、`config`、`widget` 等。
- 开源源码 `app/Controller/Shared/Commodity.php` 和 `app/Util/SharedPayload.php` 进一步确认目录输出包含 `cover`、`description`、`code` 等白名单字段，列表只输出开放 API 对接且上架的商品。分类和商品的状态过滤由源码确认。
- 官方文档未给出商品图片 CDN 生命周期/防盗链承诺；TG-Card 应下载到自己的 DATA_DIR 并校验 MIME、大小和 URL 安全性，具体可下载性待实测。

### 3.3 价格、库存和下单

- `POST /shared/commodity/inventoryState`：`shared_code`、`num` 必填，可带 `card_id`、`race`。
- `POST /shared/commodity/inventory`：使用 `sharedCode`；可返回 `count`、`delivery_way`、`draft_status`、`price`、`user_price`、`config`、`factory_price`、`is_category`。
- `POST /shared/commodity/valuation`：按 `code`、`num` 和可选 SKU/种类/预选卡询价。
- `POST /shared/commodity/trade`：必须传 `shared_code`、`num`；可传 `request_no`、`contact`、`race`、`sku`、`card_id`、`password`、`coupon`、`device` 和商品 widget 要求的字段。接口强制从上游账户余额扣款。
- 返回字段包括 `tradeNo`、`amount`、`secret`、`stock`；`secret` 可能是 null 或手动发货等待提示，不能一律视为已交付卡密。
- 建议传唯一 `request_no`，作为上游幂等请求号。

### 3.4 查单与余额

- `POST /shared/commodity/query`：必须传 `tradeNo`（注意大小写），返回 `secret`、`widget`、`status`（0 未支付、1 已支付）。
- 手动发货是否会在该接口中返回最终交付内容、状态字段与 `secret` 的对应关系，待测试账号验证。
- 官方公开 API 文档没有列出余额查询接口；`/shared/authentication/connect` 仅作为鉴权连通测试说明，不能据此声称可查余额。

## 4. 萌次元（MCY Shop）核实结论

- 官方文档入口为 `https://wiki.mcy.im`，本次访问返回 403，未能直接读取开发者货源 API 规范。
- 官方开源仓库 `lizhipay/mcy-shop` 的 README 说明其基于异次元 3.0 重构，并指向上述 Wiki。
- 源码 `app/Controller/User/API/Shop/Supply.php` 展示货源列表、详情、交易等用户 API：列表使用分页，包含货源商品、分类和 SKU 图片/价格字段；交易以 `repertory_item_sku_id`、`quantity` 进行采购。
- 这些 API 使用 `PostDecrypt`、`Waf`、`User`、`Merchant` 拦截器及登录会话上下文。列表可见性还依赖公开状态、货源对接码和会话授权。源码并未证明外部站点可仅凭“站点 URL + 商户 ID + 商户密钥”调用。
- MCY 仓库 README 还包含商业使用限制声明；在正式商业接入前需由项目所有者核对当前授权/商业许可要求。
- 结论：不根据 ACG-Faka 协议猜测 MCY 协议，不实现未经确认的 API 请求。需要 MCY 官方提供可供外部商户调用的接口文档、鉴权流程、目录/采购/查单字段和商业使用许可确认后再建立 adapter。

## 5. Dujiao-Next 协议

### 5.1 鉴权

- Base URL：`https://<upstream-site-domain>/api/v1/upstream`。
- 请求头：`Dujiao-Next-Api-Key`、`Dujiao-Next-Timestamp`（Unix 秒）、`Dujiao-Next-Signature`（小写 hex）。
- 签名串：`{METHOD}\n{PATH}\n{TIMESTAMP}\n{BODY_MD5}`。
- 签名：`hex_lower(hmac_sha256(signString, api_secret))`；BODY_MD5 是请求 body 的 MD5，小写 hex；空 body 使用空字符串 MD5。
- 时间戳允许 ±60 秒。API Key 必须已批准、启用，关联用户必须有效。

### 5.2 目录

- `GET /categories`：返回 `categories[]`，字段有 `id`、`parent_id`、`slug`、多语言 `name`、`icon`、`sort_order`；最多两层分类。
- `GET /products?page&page_size`：分页，默认 page=1/page_size=20，page_size 1～100；仅返回上架商品。
- 商品字段：`id`、`slug`、多语言 `title`、`description`、`content`、`images[]`、`price_amount`、可选 `original_price`/ `member_price`、`fulfillment_type`、`is_active`、`category_id`、`skus[]`。
- SKU 字段：`id`、`sku_code`、`spec_values`、`price_amount`、`stock_status`、`stock_quantity`、`is_active`。无限库存用 -1 表示。
- 商品图片是 URL 数组；落盘下载需增加 URL 白名单/私网地址拦截，避免 SSRF。

### 5.3 采购、查单、回调和余额

- `POST /orders` 请求字段：`sku_id`、`quantity`，可选 `manual_form_data`、`downstream_order_no`、`trace_id`、`callback_url`。
- 同一 API Key + `downstream_order_no` 重复请求会返回已有订单，适合作为 TG-Card order ID 的幂等键。
- 创建采购单时上游会自动从对接钱包扣款；钱包余额不足或支付失败返回 `ok:false`，业务错误可能使用 HTTP 200。
- `GET /orders/:id` 返回订单状态；已交付时 `fulfillment.payload` 是卡密等纯文本，另可返回 `delivery_data`。
- 上游还可通过 `POST /upstream/callback` 推送订单状态和交付内容。回调需按同一 HMAC 签名规则验签；重复回调必须幂等。
- `POST /ping` 返回对接钱包 `balance` 和 `currency`，可作为余额查询。
- 官方说明采购在支付成功后异步执行；采购失败应保留异常供管理员处理，不应自动取消本地订单或退款。TG-Card 的内部状态机必须保留“买家已付款、上游采购失败”这一事实。

## 6. 统一 Go 适配器契约

接口定义位于 `internal/upstream/adapter.go`，当前只定义 provider-neutral 类型和四项能力，不包含猜测性网络协议实现。

```go
type Adapter interface {
    Provider() Provider
    SyncCatalog(ctx context.Context) (Catalog, error)
    CreateOrder(ctx context.Context, request PurchaseRequest) (PurchaseResult, error)
    GetOrder(ctx context.Context, externalOrderID string) (OrderResult, error)
    GetBalance(ctx context.Context) (Balance, error)
}
```

设计约束：

- ACG 与 Dujiao 的 HTTP、签名、字段转换必须留在各自 adapter 包内；不能把上游字段直接泄漏到 TG-Card 的商品/订单 API。
- MCY 在外部商户协议确认前不提供可用 adapter 实现；若某能力没有文档/源码依据，应返回明确的 unsupported 错误，不得伪造 endpoint。
- `Product.Stock == nil` 表示上游没有提供可确认的库存数，不得转换成 0；价格使用十进制字符串传输，避免 float 精度问题。
- `PurchaseRequest.DownstreamOrderID` 是 TG-Card 本地订单号；各 adapter 将其映射到上游支持的幂等字段（ACG: `request_no`；Dujiao: `downstream_order_no`）。
- `PurchaseResult.Pending` 表示需要查单/回调后才能交付；收到空内容或“处理中”占位内容不得当作成功发货。
- `Metadata` 仅用于保存经过审核的上游扩展参数；密钥、签名和原始认证头不得放入 Metadata 或日志。

## 7. 实施顺序与未确认事项

1. 阶段 A（本 PR）：完成调研矩阵、接口契约、协议风险记录。
2. 阶段 B：实现配置加密、数据迁移、ACG/Dujiao 目录同步、图片本地化和来源过滤；MCY 暂不纳入，直到协议核实。
3. 阶段 C：在买家付款确认之后执行异步上游采购、幂等、查单/回调、失败队列、人工重试和站内余额退款。
4. 阶段 D：各上游测试账号验收目录同步、采购、交付、失败重试；生产真实订单由项目所有者验收。

### 明确待确认

- ACG：图片 URL 长期可访问性；手动发货的异步状态和最终 secret 语义；无已知余额查询端点。
- MCY：官方公开商户 API、认证/签名、分类与商品接口、价格/库存字段、采购接口、查单接口、余额接口、回调能力和商业授权条件。
- Dujiao：多币种价格与 TG-Card 当前订单币种换算策略；人工交付商品的异步采购验收；回调公网地址与验签部署条件。
- 所有供应商：API 限频、最大响应体、图片大小限制、订单保留期与重复请求时效，待供应商确认/测试。

## 8. 官方文档与源码引用

### 异次元发卡
- 官方 API 文档：<https://faka.wiki/zh-cn/dev/api.html>
- 商品共享控制器源码：<https://github.com/lizhipay/acg-faka/blob/main/app/Controller/Shared/Commodity.php>
- 商品字段出站白名单：<https://github.com/lizhipay/acg-faka/blob/main/app/Util/SharedPayload.php>
- 共享签名实现：<https://github.com/lizhipay/acg-faka/blob/main/app/Service/Bind/Shared.php>

### 萌次元
- 官方 Wiki：<https://wiki.mcy.im>（本次读取 403）
- 官方仓库：<https://github.com/lizhipay/mcy-shop>
- 货源用户 API 源码：<https://github.com/lizhipay/mcy-shop/blob/main/app/Controller/User/API/Shop/Supply.php>
- 商业许可声明见仓库 README；商业接入前须核对。

### Dujiao-Next
- 官方 Open API：<https://dujiao-next.com/api/integration-open-api>
- 官方站点对接说明：<https://dujiao-next.com/api/integration-guide>
- 官方开源仓库：<https://github.com/dujiao-next/dujiao-next>
