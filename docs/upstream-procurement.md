## 2026-11-22 — PR #129 manual 商品买家单件自助购买（已合并并发布；PROJECT_STATUS 2.2.82）

- PR #129 已 Squash Merge，merge commit `8a55d5959bb30bbec03006e0121298dc51c6294f`；新增 Telegram 买家动态表单采集，manual 表单只允许私聊填写，支持 20 分钟会话过期、`/cancel` 与机器人重启后恢复。
- 表单会话及订单最终表单快照均以 AES-GCM 密文持久化；采购 worker 仅对 manual 履约订单解密并转发 Dujiao-Next `manual_form_data`。数据缺失、解密失败或 JSON 格式异常时 fail closed，阻止采购提交。
- auto 履约不携带 `manual_form_data`，既有 auto 采购与本地商品购物车流程不变。
- 当前只开放单件直购；manual 商品暂不支持购物车和优惠码。未进行真实商户联调，生产环境仍需单独验收数据库迁移与 Telegram 实际购买流程。
- 合并后 Go/Admin/Security/Docker Publish 均成功；GHCR OCI digest：`sha256:a1e26577afb22d7d285b4d69dfc7f61d235eb26c70a26122cee29dbaa4e47c02`。

## P0-4 补充 — uncertain 采购人工核实后台入口

- 上游采购人工处理队列对 `uncertain` 状态只展示“核实受理状态”，不再展示直接重试或退款按钮。
- 管理员必须先在供应商后台按本地订单号核实：确认已受理时填写供应商订单 ID，系统恢复 polling 并只执行 GET 查单，不再次 POST 创建采购单；确认未受理/明确失败时记录核实依据，将队列收敛为 failed，之后才允许沿用原本地订单号重试或退款。
- 两种人工决策均要求填写核实说明，并记录采购事件与管理员审计；不会自动改余额、退款或重新创建供应商订单。
- 本次复用既有 `GET /api/upstream/procurements/uncertain` 与 `POST /api/upstream/procurements/{id}/resolve` 接口，仅补齐上游设置页操作入口；无数据库迁移或新配置。

## P0-4 补充 — 上游采购受理不确定时禁止重试与退款

- `upstream_procurements.status='uncertain'` 表示采购请求可能已被供应商受理，但本地未获得可确认响应；不得将其当作普通采购失败处理。
- 对应本地订单虽显示 `procurement_failed`（用于进入人工处理队列），仍禁止管理员直接重新排队，也禁止向买家钱包退款。
- 只有供应商侧明确确认未受理/失败、采购队列状态已收敛为 `failed` 后，才允许使用原本地订单号幂等键手动重试或退款。
- 当前上游接口未提供按下游订单号可靠查单能力时，uncertain 订单必须由 Owner/管理员先在供应商后台核实；本次不增加自动重试、不自动退款、不修改钱包账本。
- 新增回归测试覆盖 uncertain 订单重试与退款均被拒绝，且上游创建请求次数、订单状态和钱包流水保持不变。
- 无数据库迁移、无新增环境变量/后台配置；既有 uncertain 订单升级后自动受到该保护。

## P0-4 补充 — 已退款上游订单缺失采购记录

- 对来源快照明确为 upstream 的订单，若本地订单状态为 `refunded`，退款完整性核对还必须确认存在 `upstream_procurements` 生命周期行。
- 若该行缺失，系统无法核对供应商侧受理/失败/退款状态，因此登记 critical 财务异常，交由 Owner/管理员核实。
- 该检测只读，不自动补建采购行，不创建退款流水，不修改订单/钱包，不触发上游 API。

## P0-4 补充 — 退款状态双向一致性核对

- 退款完整性检查不仅核验退款账本、钱包 credit、金额、买家与管理员关联，还核验订单与采购队列的退款状态一致性。
- 合法完成退款必须同时满足 `orders.status='refunded'` 与 `upstream_procurements.status='refunded'`。
- 若订单已退款但采购队列仍为 failed/uncertain，或采购队列已退款但订单不是 refunded，均登记为财务异常中心的 critical 异常，按订单维度幂等归档。
- 此检查为只读检测，不自动补写状态、不增减钱包余额、不自动创建退款流水；必须由 Owner/管理员核实底层退款交易和订单记录后人工处理。

## P0-4 补充 — 交付结果不确定通知

- 本地 Telegram 交付请求一旦发出，若调用返回超时/网络错误，订单必须保持 `delivery_uncertain`，不能据此认定客户未收到，也不能自动盲目重发。
- 系统通过 `finance_exception_alerts` 对 `delivery-uncertain:<order-id>` 使用固定 `once` 告警键，保证同一订单只认领一次 Owner 告警；通知复用站点 `risk_alert` Telegram 配置。
- Owner 通知仅提供订单号、错误原因和人工核实提示，不发送卡密、交付内容或密钥。
- 管理员应先通过 Telegram 实际消息/客户反馈核实是否送达，再按订单详情中的步骤状态执行有审计的处理。通知发送失败不改变订单状态，也不触发退款或余额修改。

## P0-4 — 已付款订单缺失采购队列恢复（已合并）

- 正常付款路径在同一数据库事务中更新订单为 `procurement_pending` 并插入 `upstream_procurements`。若历史数据/异常中断导致订单仍为该状态但队列行缺失，调度器会周期性扫描并恢复队列。
- 恢复范围严格限定：订单状态必须仍为 `procurement_pending`、存在 `paid_at`，且订单来源快照或履约类型明确为 `upstream`；已存在采购记录的订单不重复创建。
- 恢复任务沿用本地订单 ID/订单号；deadline 使用原始付款时间加当时配置的采购超时，不会因重启/补偿重新计时。
- 不自动重排 `procurement_failed`、`uncertain`、`refunded` 或 `delivered` 订单；这些状态需要既有查单、Owner 核实或人工售后流程。
- 本恢复器只补建采购队列，不触发钱包扣款/credit、不改支付记录、不修改供应商余额；上游请求仍由既有 worker 在重新核对订单与商品状态后执行。
- 自动化验证：SQLite 测试覆盖缺失队列恢复、重复扫描幂等、订单支付记录不变及钱包账本不变；PostgreSQL 16 CI 覆盖待本 PR 检查结果确认。

## PR U 目录同步变更边界（2026-11-20）

- PR U #56 独立处理目录分页同步、Bot 来源过滤分页与上游目录停用清理；Bot 的 auto 履约商品会进入现有订单/付款/采购队列，不改变 PR C/D 的采购扣款、采购幂等、退款账本或资金对账逻辑。manual 履约商品仍不开放自助下单。
- 上游商品/分类目录清理只删除 `source_type='upstream'` 数据；订单通过名称/金额快照保留历史可读性，采购事件、退款行和资金流水继续引用订单，不因商品目录清理而删除。
- PR U #56、PR S #57 均已合并并发布 GHCR。Dujiao-Next 正常对接已由项目所有者确认完成；不再要求额外 PR E 真实商户验收或压力测试。

# Dujiao-Next 采购转发与订单履约（PR C）

> 当前对应 PR #32：\`feat/dujiao-purchase-forwarding\`。本文记录 PR C 的代码实现范围与运行约束。PR C 已通过独立 PR #32 合并；PR D 已通过独立 PR #38 合并并发布 GHCR；项目所有者于 2026-11-22 确认 Dujiao-Next 正常对接完成，不再要求 PR E 真实商户联调；本节保留 PR C 的实现和 mock 测试事实，不把 mock 覆盖范围扩大为额外验收要求。

## 1. 订单状态与触发时机

1. 买家在 TG-Card 创建本地订单时，订单快照保存上游来源、上游商品 ID、SKU ID、履约类型。
2. TRC20 支付确认或站内余额付款，在同一数据库事务中将订单置为 \`procurement_pending\` 并写入 \`upstream_procurements\` 队列；队列按本地订单 ID 和订单号唯一。
3. worker 领取任务后重新读取订单状态与商品状态。仅当订单仍为 \`procurement_pending\`、商品仍上架且来源仍为 upstream 时才调用上游；已取消/已退款/商品下架/来源变更均不采购。
4. 上游交付内容加密保存到订单交付字段，然后使用现有 Telegram 交付租约和发货逻辑。

## 2. 幂等和订单映射

- \`downstream_order_no\` 固定使用 TG-Card 本地订单号，不生成重试新号。
- \`upstream_procurements.local_order_id\` 是主键，\`local_order_no\` 唯一；\`external_order_id\` 有非空唯一索引，另存 \`external_order_no\`。
- 支付重复通知只会触发同一采购队列行，不会创建第二条任务。
- 上游创建成功后先持久化上游订单 ID/订单号，再进入查单；不会依赖内存状态维持订单映射。

## 3. 网络异常与查单策略

官方 Dujiao-Next Open API 当前公开：
- \`POST /api/v1/upstream/orders\`：创建采购单，支持 \`downstream_order_no\` 幂等；相同 API Key + 相同下游订单号重复提交会返回已有订单。
- \`GET /api/v1/upstream/orders/:id\`：按上游订单 ID 查单，已交付时包含 \`fulfillment\`。
- 未公开按 \`downstream_order_no\` 查询订单的 GET 接口。

因此：
- POST 成功取得 \`order_id\`：保存上游映射；非终态订单按后台配置轮询。
- 已取得上游订单 ID：后续先 GET 查单；已交付则交付，已取消则失败，其他状态等待下一轮。
- 首次 POST 超时/网络异常且没有取得 \`order_id\`：标记 \`uncertain\`、订单标记 \`procurement_failed\`、通知 Owner 并停止自动重发。不能调用未公开的查询接口，也不能盲目换新幂等号。
- 管理员在核实上游订单后可手动重试；重试仍使用原 \`downstream_order_no\`。上游已受理时，官方幂等规则应返回已有订单。

## 4. 异步回调

回调端点：\`POST /api/v1/upstream/callback\`

验证：
- \`Dujiao-Next-Api-Key\` 与当前配置 API Key 恒定时间比较。
- \`Dujiao-Next-Timestamp\` 为 Unix 秒，允许时钟偏差 ±60 秒。
- 按官方规范对原始请求体计算 MD5，再构造 \`POST\\n{path}\\n{timestamp}\\n{body_md5}\`，用 API Secret 进行 HMAC-SHA256，并恒定时间比较签名。
- 回调业务内容计算稳定 SHA-256 幂等键；同一事件重复投递只记录一次。
- 验签失败返回 401 并通过 Owner Telegram 风险通知；不得修改订单或交付。

状态：
- \`delivered\` / \`completed\`：要求有 \`fulfillment.payload\` 或 \`fulfillment.delivery_data\`，加密保存并进入本地 Telegram 交付。
- \`canceled\`：标记采购失败并进入人工队列。
- 其他状态：保存映射并继续查单轮询。
- 已退款/已交付订单不会被迟到回调重新交付。

## 5. 失败队列、重试和退款

采购失败包括上游业务拒绝（余额不足、SKU/商品不可用、库存不足）、上游订单取消和查单超时。失败时：
- \`orders.status=procurement_failed\`，错误原因写入 \`procurement_error\` 与采购事件表。
- 不修改原支付方式、交易引用、支付金额和支付确认记录；不自动退款。
- Owner 通知包含本地订单号和订单入口。
- 后台上游页面列出采购队列，支持手动重试和退款。

退款：
- 仅允许 \`procurement_failed\` 且采购状态为 failed/uncertain 的订单。
- 管理员填写退款原因；事务内给买家钱包增加 credit，插入 \`upstream_procurement_refunds\` 独立账本，更新订单为 refunded，写采购审计事件。
- 管理员操作写 \`admin_audit_logs\`；原始支付记录保留不变。
- 退款 credit 纳入每日退款统计；余额对账公式按钱包 credit - debit 计算。

## 6. 供应商余额监控

- 每 10 分钟检查一次上游余额（配置启用时）。
- 余额快照写入 \`upstream_balance_snapshots\`。
- 后台可配置低余额阈值；阈值为 0 表示关闭告警。
- 低余额告警按 UTC 日期去重，并通过 Owner Telegram 通知。
- 每日对账报告保存该 UTC 日内最近一次供应商余额、币种和检测时间；历史日报页面展示余额字段。

## 7. 后台配置

- 回调地址：HTTPS 公网地址，路径固定 \`/api/v1/upstream/callback\`。
- 查单间隔：5–3600 秒，默认 30 秒。
- 采购超时：1–1440 分钟，默认 30 分钟。
- 供应商低余额阈值：按供应商账户币种填写，0 关闭。
- 不需要新增环境变量；设置持久化在 \`upstream_settings\`。

## 8. 当前限制与阶段边界

- manual 商品单件自助购买已由 PR #129 合并并发布：Telegram 买家动态填写上游 schema 字段，私聊会话与最终订单表单数据均以 AES-GCM 密文持久化；付款后仅 manual 订单向上游传入 `manual_form_data`。
- 当前仅支持单件直购，不接入购物车；优惠码暂不支持 manual 商品。auto 商品继续沿用原采购流程。
- PR #129 的 Go/Admin/Security CI 与合并后 main 的 Go/Admin/Security/Docker Publish 均已通过，镜像 digest 见本文顶部；生产迁移执行与真实 Telegram 买家购买流程仍待目标环境验收。
- PR C 使用 mock/fake API，不需要真实商户凭证。
- PR D（PR #38）已实现进一步供应商资金安全、扣款/余额一致性与异常保护，并已合并发布；项目所有者于 2026-11-22 确认 Dujiao-Next 正常对接完成，不再要求额外 PR E 真实商户联调。
- PR #38 已合并，merge commit `935bd222977b1a636c5f05ab4faebdc3e835dda1`；main Go/Admin/Security/Docker Publish 均成功，GHCR digest `sha256:691493ce844c96a504785e0fb72b24534786f57279190322332f45b5ba8b7b29`。Dujiao-Next 正常对接已由项目所有者确认完成；不再将 PR E 真实商户联调列为项目待办。
- 异次元 ACG-Faka、萌次元 MCY 继续不开发适配器。

## 9. Mock 测试覆盖

\`cmd/server/upstream_procurement_test.go\` 使用可注入 \`ProcurementAPI\` 假上游，覆盖：
1. POST 同步返回交付内容；
2. POST 返回处理中、GET 查单后交付成功；
3. 查单超过 deadline 转采购失败人工队列；
4. 上游业务拒绝（模拟余额不足）；
5. 相同回调重复投递只交付一次；
6. 回调签名错误被拒绝；
7. POST 请求响应丢失后进入 uncertain，人工重试沿用原下游幂等键并解析为同一上游订单；
8. 已取消/已退款本地订单在采购前被跳过并审计；
9. 上游 HTTP 402 余额不足响应被解析为明确业务失败并保留上游订单 ID；
10. 上游 HTTP 404 查单响应被解析为 `order_not_found`，不会当作网络超时盲目重试。

各测试检查订单状态、原始支付引用、钱包流水未被采购失败/重试修改，以及采购/回调审计记录。PR #32 的 PR head 与合并后 Actions 结果记录在 `docs/PROJECT_STATUS.md` 第 19.7 节。

## 10. PR D 资金安全深化（PR #38 已合并并发布，2026-11-20 状态校准）

### 10.1 上下游采购资金对账
- 采购创建请求前读取供应商余额并保存采购专属 baseline；请求返回后读取第二次余额快照。
- 以本地订单 ID/订单号关联供应商返回订单 ID、采购金额、采购前余额、采购后余额，计算观测扣款与预期采购金额差异。
- 余额快照缺失、币种不一致、金额不可解析、对照超时或差异超过 0.01（供应商币种最小单位）进入异常中心并通知 Owner。
- 同一订单的第一次 baseline 在幂等重试时保留，不用重试前余额覆盖原始快照。供应商同时发生其他扣款/充值时，余额差异可能包含外部活动；因此仅供人工核查，不自动冲账。
- 定时巡检把超过 15 分钟仍无采购后余额快照的记录标记为 unverifiable 并进入异常中心。

### 10.2 退款闭环审计
- 退款账本核对订单 refunded 状态、原始订单金额、退款记录金额、管理员 ID、原因、唯一 wallet credit 流水、买家 ID 与 credit 金额。
- 检查已退款采购订单缺少退款行、退款 credit 孤立等异常；只新增异常记录，不修复钱包余额或流水。
- 退款事务再次检查订单退款状态与 refund row；order_id 主键和 wallet_transaction_id 唯一约束确保每单仅一笔退款。买家余额 credit、独立退款行、订单状态、采购事件与管理员 audit log 在同一数据库事务中提交；任何一步失败均整体回滚。退款成功后原支付方式/交易引用不改写。

### 10.3 uncertain 订单治理
- 独立接口列出 uncertain 订单并返回等待秒数/ageing。
- Owner 核实供应商已受理时必须填写正整数上游订单 ID，系统延长查单 deadline、恢复 polling 并继续 GET 查单（不重新 POST）；核实未受理时记录人工说明，将采购队列置为 failed，随后可按原本地订单号幂等重试或执行退款。
- 所有人工核实动作写采购事件及管理员审计；不得仅凭“猜测未受理”自动重发。

### 10.4 统一资金异常中心
- 统一分类：procurement_failed、procurement_uncertain、supplier_balance、refund_ledger。
- 异常记录含严重级别、关联订单/实体、详情、处理状态、处理说明、操作管理员与时间；提供待处理数量、筛选、详情和状态更新。
- 异常状态变更写入管理员审计；待处理数量作为后台红色提示。PR D 合并/CI/镜像/部署/验收状态分别记录。
- 所有巡检只读；异常中心不提供自动余额修正、自动退款或自动供应商补款入口。

