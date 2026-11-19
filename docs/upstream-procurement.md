## PR U 目录同步变更边界（2026-11-20）

- PR U #56 独立处理目录分页同步、Bot 来源过滤分页与上游目录停用清理；Bot 的 auto 履约商品会进入现有订单/付款/采购队列，不改变 PR C/D 的采购扣款、采购幂等、退款账本或资金对账逻辑。manual 履约商品仍不开放自助下单。
- 上游商品/分类目录清理只删除 `source_type='upstream'` 数据；订单通过名称/金额快照保留历史可读性，采购事件、退款行和资金流水继续引用订单，不因商品目录清理而删除。
- PR U 当前为 Ready for review；Go 测试、Admin 构建与 Security 均已成功；未合并、未发布镜像、未部署。真实 Dujiao-Next 采购、供应商扣款、回调可达性与失败退款验证仍属于 PR E，由项目所有者在真实环境执行。

# Dujiao-Next 采购转发与订单履约（PR C）

> 当前对应 PR #32：\`feat/dujiao-purchase-forwarding\`。本文记录 PR C 的代码实现范围与运行约束。PR C 已通过独立 PR #32 合并；PR D 已通过独立 PR #38 合并并发布 GHCR，生产部署/验收待确认；PR E 真实商户联调尚未开始。不得将 mock 测试等同于真实上游联调。

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

- 当前仅支持上游 \`auto\` 自动交付 SKU。
- \`manual\` 商品需要 \`manual_form_data\`；买家侧动态表单采集尚未实现，因此 Telegram 商城暂不允许自助购买这类商品。
- PR C 使用 mock/fake API，不需要真实商户凭证。
- PR D（PR #38）已实现进一步供应商资金安全、扣款/余额一致性与异常保护，并已合并发布；生产部署/验收仍待确认。
- PR #38 已合并，merge commit `935bd222977b1a636c5f05ab4faebdc3e835dda1`；main Go/Admin/Security/Docker Publish 均成功，GHCR digest `sha256:691493ce844c96a504785e0fb72b24534786f57279190322332f45b5ba8b7b29`。生产部署/验收尚未确认。PR E 才进行真实商户连接、供应商实际扣款、回调公网可达、交付和失败退款全链路联调。
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

