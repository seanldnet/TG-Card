## 2026-11-19 — Dujiao-Next PR D 资金安全深化（PR #38 Draft，分支 CI 通过）

- PR：[PR #38](https://github.com/seanldnet/TG-Card/pull/38)，独立分支 feat/dujiao-fund-safety-pr-d，head 5dbd0269a719f91b74458f03bd67a25aeab0761b，基线 main@0b0f0514a1c5850aafec0018c95570d0bce8a39f。
- 状态：Go CI #44370639264、Admin CI #44370639142、Security #44370639404 均成功；PR #38 仍为 Draft、尚未合并；分支 CI 通过，Docker Publish/生产部署/验收待后续。
- 实现：供应商采购前后余额快照和差异异常、15 分钟未完成快照巡检、退款账本完整性检查与重复退款保护、uncertain ageing/Owner 核实恢复、统一资金异常中心及处理状态。
- 测试：go vet ./...、go test ./...、CGO_ENABLED=0 go build 全部成功；SQLite 测试覆盖供应商资金差异、退款关联异常不自动修正、重复退款仅一笔 credit、uncertain ageing 和 Owner 恢复。
- 资金边界：不自动修改买家或供应商余额；采购失败/重试不改原始支付引用；退款单订单唯一；供应商差异只进入人工审查。PR E 真实供应商资金联调未完成。

## 2026-11-19 — Dujiao-Next PR D 资金安全深化（独立分支开发中）

- 状态：独立分支 feat/dujiao-fund-safety-pr-d 基于 main 0b0f0514a1c5850aafec0018c95570d0bce8a39f；正在实现，PR 尚未创建，CI/镜像/生产部署/验收均未完成。
- 仓库审查：PR2 只读校验覆盖买家余额流水；PR C 退款有唯一退款行与唯一钱包流水关联，但缺少完整性巡检；供应商余额快照尚未按采购逐笔与扣款金额比对；uncertain 缺少 ageing/独立人工核实；无统一资金异常中心。
- 当前代码变更：新增迁移 0039_finance_safety.sql、供应商采购前后余额对照、退款账本一致性检查、uncertain ageing 与 Owner 核实 API、统一资金异常 API/后台页面和定时巡检。检测只生成异常与通知，不自动调整余额。
- 新增 SQLite 测试覆盖供应商余额差异、退款账本损坏检测、重复退款拒绝、uncertain ageing/Owner 恢复；需以 PR Actions 最终结果为准。
- 资金红线：退款仅一条独立账本并关联唯一 wallet credit；采购重试沿用原本地订单幂等键；采购失败不改变原始支付记录；不自动修改买家或供应商余额。
- PR E 真实供应商联调仍未完成；PR #27 C1–C4 继续待验收。

## 2026-11-19 — PR B 上游商品同步验收通过

- 项目所有者确认后台绑定商户、目录同步、加价展示和来源切换均正常；PR B（PR #26）D1–D5 验收状态更新为已验收，日期 2026-11-19。
- 验收镜像仍为生产 PR2 digest sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e。PR #27 C1–C4 未测试，保持待验收。

## 2026-11-18 — Dujiao-Next PR C 采购转发（PR #32 已合并，CI/Mock 通过）

- 状态：独立 PR #32 已合并，merge commit `4f712e6ecb2299abd1eb782a439dee56b760578c`；PR head Go/Admin/Security 与 main Go/Admin/Security/Publish 全部成功；十项 mock 测试通过。未执行真实上游联调。
- 分支：`feat/dujiao-purchase-forwarding`，基线 `main@2e7f91cfe772c3a87b3c0c40d8542bff59b9c9be`。
- 实现：支付确认事务登记采购任务；采购前复核订单/商品；本地订单号幂等创建、上游订单 ID/订单号映射；查单轮询、回调验签/幂等；加密交付；失败人工队列、手动恢复、站内余额退款独立账本与审计；供应商余额快照/阈值告警/日报。
- 资金安全：采购失败和重试不修改原支付记录、不自动退款买家余额；管理员手动退款使用独立钱包 credit 流水和退款审计。
- 协议限制：官方 API 未提供按 `downstream_order_no` 查单接口。首次 POST 网络异常且未获得上游订单 ID 时进入 `uncertain`，暂停自动重发；人工恢复仍使用原幂等键。已获得上游订单 ID 时才调用 `GET /orders/:id`。
- 当前限制：后台必须配置公网 HTTPS 回调地址才允许上游自助下单；只开放 auto 自动交付 SKU，manual 表单 SKU 暂不开放。真实商户端到端联调留给 PR E。
- Mock 测试：同步交付、异步查单成功、查单超时、采购失败、重复回调、验签失败、超时后同幂等键恢复、已退款订单跳过、HTTP 402/404 错误分类共 10 项通过。
- PR：[PR #32](https://github.com/seanldnet/TG-Card/pull/32)。PR #32 merge Publish digest：`sha256:206c0ca3a12526aaeeb6ec85e17adbbc155803a8f3dc64d9f76667ac8f1fa0cf`；main 重建 Publish Run #44338105044 成功，当前 latest digest：`sha256:1228783d3d8c9c7fab302cc86f60a17866c783d52cf26f27000bcc56e2a55be8`。生产验收仍按 PR2 digest `sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e` 记录；PR D 与 PR E 仍未完成。

## 2026-11-19 — PR2 生产验收通过；Dujiao-Next PR C 已合并

- 状态：PR #30（PR2）已合并至 main；Go CI、Admin CI、安全扫描、Docker Publish 全部成功。项目所有者确认生产已更新至 PR2 镜像，PR #19、#26、#28 与 PR2 验收按默认通过处理，验收日期 2026-11-19。
- PR2 merge commit：`75a635a6c5e9c68e8fc23c229f932119dca46a93`；Publish Run [#44324284760](https://github.com/seanldnet/TG-Card/actions/runs/44324284760)；GHCR multi-platform manifest digest：`sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e`。
- Go CI [#44324284650](https://github.com/seanldnet/TG-Card/actions/runs/44324284650)、Admin CI [#44324284642](https://github.com/seanldnet/TG-Card/actions/runs/44324284642)、Security [#44324284777](https://github.com/seanldnet/TG-Card/actions/runs/44324284777) 均成功。
- PR #19/#26/#28 与 PR2 的本次验收项均使用生产 PR2 digest `sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e` 记录为已验收。PR #27 Owner Bot C1–C4 未被本次明确列入验收范围，继续待验收。
- 采购转发、供应商扣款、异步查单/回调、失败人工队列、退款审计现进入独立 PR C；PR D 资金安全与 PR E 真实上游联调仍未完成。

## 2026-11-18 — PR2 资金与库存安全（已合并、发布并验收）

- 状态：PR #30 已合并至 main，Go CI、Admin CI、安全扫描、Docker Publish 均成功；生产验收由项目所有者按默认通过确认，日期 2026-11-19。
- 实现范围：UTC 每日对账日报及历史快照；用户余额与钱包流水只读一致性检查；本地卡密商品库存归零自动下架、补货后按设置手动/自动上架。
- 数据库：新增迁移 `0037_reconciliation_inventory.sql`，记录交付失败事件、日报快照、余额检查运行和异常明细。
- API/后台：新增对账日报月度查询、余额检查历史/差异查询、手动只读检查接口；新增“对账与余额校验”页面；网站设置新增库存归零下架/补货自动上架开关。
- 调度：日报每日 UTC 00:05 后生成前一日快照；余额校验启动时执行并每 15 分钟触发、按小时幂等；库存每 5 分钟巡检。
- 统计口径与资金边界：日报按 paid_at/created_at/delivered_at/失败事件/退款 credit/充值本金/链上时间统计。余额只读核验按 wallet_transactions credit - debit；售后退款在当前实现中是 credit，故计入正向流水。任何差异均只记录和告警，不自动修正余额。
- 验证：新增日报指标、快照幂等、余额差异检测且不自动修正的 SQLite 测试；Go CI #44324284650、Admin CI #44324284642、Security #44324284777、Publish #44324284760 均成功。
- 文档：同步更新 `docs/PROJECT_STATUS.md`、本日志与新增 `docs/reconciliation-inventory.md`。
- 验收：项目所有者确认生产已更新至 `ghcr.io/seanldnet/tg-card@sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e`，未收到异常反馈，按默认通过处理（2026-11-19）。
- 红线：本 PR 不包含 Dujiao-Next 采购转发、供应商扣款或退款逻辑；不自动改余额。

## 2026-11-18 — PR #29 交接文档校准与 Admin CI 触发修复

- 状态：已合并至 main，merge commit `55b09352ad902cfaf97f323300eed6b7183e55cf`。
- 实现范围：校准 PR #28 合并/CI/GHCR digest 证据；修正 PROJECT_STATUS 元信息与追踪表；修正 Dujiao-Next PR B 已合并状态及 PR C/D/E 待开发边界；增加 18 项统一生产验收表；售后专项标注 PR #28 已合并。
- 根因：Admin frontend CI 原先对 workflow、`web/**`、`scripts/**`、README 使用 paths 过滤，PR #28 未改这些路径而没有 Admin CI run。
- 修复：移除 Admin CI 的 paths 过滤，所有 main push 与面向 main 的 PR 均触发。PR #29 Admin CI [Run #44321310277](https://github.com/seanldnet/TG-Card/actions/runs/44321310277) 成功。
- 验证：文档与 workflow 修改已合并；该 PR 不改业务代码，不触发新的业务镜像发布。PR2 必须核对 Go、Admin、Security、Publish 四类 Actions。
- 验收：PR #19、#26、#27、#28 真机项目仍待项目所有者按 PROJECT_STATUS 第 19.3 节逐项回填。

## 2026-11-18 — PR #28 售后结单闭环发布核验与后续路线修订

- 状态：PR #28 已合并至 main；Go CI、安全扫描、Docker 多架构镜像均成功；生产 Telegram 验收待项目所有者执行。
- PR / Merge commit：[PR #28](https://github.com/seanldnet/TG-Card/pull/28)，`6bbb749e0c89f95ee5e37aa21d92a14bd916a6c6`。
- Publish：[Run #44317232482](https://github.com/seanldnet/TG-Card/actions/runs/44317232482)，completed/success；GHCR latest 与 commit SHA 标签推送成功；manifest digest `sha256:89cd1443e63282e171c026460c545fce73e5ad9514c4b6452abe8b6b01fd1f04`；平台 linux/amd64、linux/arm64。
- Go CI：[Run #44317232633](https://github.com/seanldnet/TG-Card/actions/runs/44317232633) 成功；Security：[Run #44317232660](https://github.com/seanldnet/TG-Card/actions/runs/44317232660) 成功。
- Admin frontend CI 未运行的原因已确认：原 `.github/workflows/admin-ci.yml` 对 workflow 文件、`web/**`、`scripts/**`、README 使用 paths 过滤，PR #28 未触发这些路径。已在本次文档修正分支移除 paths 过滤，后续每个 main push/PR 均应执行 Admin CI；PR2 必须核对 Go、Admin、Security、Publish 四类结果。
- 结单闭环范围：用户结单通知、问题已解决确认、仍有问题重开、终态消息守卫、自动新工单及用户维度 10 分钟限流；迁移 `0036_support_resolution_lifecycle.sql`。
- 生产验收基准：`ghcr.io/seanldnet/tg-card@sha256:89cd1443e63282e171c026460c545fce73e5ad9514c4b6452abe8b6b01fd1f04`。PR #19、#26、#27、#28 对应真机验收统一使用该镜像；逐项清单见 `docs/PROJECT_STATUS.md` 第 19.3 节。
- 文档：更新主状态文档、上游专项边界、售后验收追踪；整理 18 项统一生产验收记录。
- 后续顺序：文档修正 → PR2 资金与库存安全 → Dujiao-Next PR C 采购转发 → PR D 资金安全 → PR E 真实联调 → PR3 异常中心 → PR4 购买体验 → PR5 数据看板 → PR6 群发精细化。
- PR 描述要求：每个 PR 写明实现范围、复现条件、验证结果和文档更新；每批独立分支，CI 通过并合并后再启动下一批。


## 2026-11-18 — 售后工单改为 Bot 转发 Owner 与交互式回复（独立 PR #27）

- 状态：已合并至 main（PR #27）；Go CI、Admin CI、安全扫描成功；Docker latest 已推送（Publish Run `44308515206`，digest `sha256:78b216140715964bf201f398e492c8f636091525a1ca327bcfaaa426532bfba2`）。真实 Telegram/SMTP 验收待项目所有者执行；后续 PR #28 最新生产验收镜像 digest 见本文顶部记录。
- 分支：`feat/support-owner-bot-workflow`；基线 `main@71c0b1b75cbe01fb0a13986cffb58cd4e394e0ee`；合并提交 `abb6d62c8d215ff4bffd4151130adccf7d346b24`。
- 影响模块：`internal/telegrambot/manager.go`、`internal/database/support.go`、`internal/database/support_owner.go`、`cmd/server/main.go`、售后后台页面、上游/支付页面布局、售后专项文档。
- 变更内容：
  - 用户创建工单和后续补充的图片/文件通过创建工单的 Bot 转发至该 Bot Owner 私聊；消息头包含工单号、用户、Bot、订单和文字摘要，并提供查看详情/待处理列表按钮。
  - Owner 待处理列表按每页 8 条分页；详情提供开始回复、结单、返回列表。
  - Owner 点击开始回复后直接发送文字/图片/文件即可回复用户；发送成功后弹出结单确认；点“否”或不点击按钮继续发消息时保持当前工单激活并再次弹出确认。
  - Owner 激活会话持久化，默认 30 分钟超时；新工单通知不抢占当前工单；重复结单不重复写审计。
  - Owner 回复失败时显示 Telegram 错误，不结单；Owner 未配置时工单继续存档并记录日志。
  - 站点 `support_created` 事件继续发送现有 SMTP 邮件和 Telegram 通知；通知失败不影响工单创建。
  - 后台工单时间线仅显示文字；附件改为 `[图片]` / `[文件]` 占位，提示在机器人查看；前端不再调用附件读取端点，旧 API 与附件字段保留。
  - 上游对接页、支付设置页改用页面级 flex column gap 统一卡片垂直间距，避免按钮区域卡片与相邻卡片贴合。
- 数据库迁移：新增 `0035_support_owner_workflow.sql`，增加 Telegram update idempotency 字段/索引、持久化 Owner 会话表和结单审计表；旧工单消息与附件字段不删除。
- 配置/部署影响：无新增环境变量；复用 Bot 现有 Owner ID、站点 SMTP 与 Telegram 通知设置；迁移由现有启动迁移器自动执行。
- 兼容性与已知限制：旧附件下载 API 保留供历史兼容，但当前后台不再请求；Telegram 媒体转发依赖创建工单的 Bot 能访问原 file_id 且 Owner 已私聊启动该 Bot。消息发送成功与时间线写入之间仍存在网络/数据库故障窗口；不宣称跨 Telegram API 的严格 exactly-once。
- 验证：
  - 自动化测试：新增 Owner 会话持久化/超时、结单审计幂等、重复 Telegram update 与重复附件去重测试。
  - Go CI、Admin CI、安全扫描均已在 PR #27 对应 Actions 运行中成功；Docker Publish Run `44308515206` 成功。
  - Telegram/SMTP/浏览器端到端：尚未执行；由项目所有者按专项验收清单执行。
- 发布镜像：已发布，digest `sha256:78b216140715964bf201f398e492c8f636091525a1ca327bcfaaa426532bfba2`；后续被 PR #28 的 latest 更新。
- 文档：同步更新 `docs/PROJECT_STATUS.md`、`docs/support-tickets.md` 与本日志；遵守第 12 节文档规则。

## 2026-11-18 — 修正 PR #26 stale 状态测试夹具

- 状态：测试夹具已修正并提交，等待最新 Go CI 复验；PR #26 未合并。
- Commit：`4575745f6f35ab0a1ebcdcf36257f63be4c034db`（测试文件提交；文档提交随后更新）。
- 根因：`SaveUpstreamSettings` 持久化调用方传入的 `CatalogStale`，不会自行推断配置是否变更；实际配置 API 在配置变更时负责将该标志置为 true。测试夹具只设置了 `Enabled=true`，却假定数据库层会自动设置 stale，导致测试误走“上游已启用且目录新鲜”的分支，空的上游活跃商品列表被错误地拿来断言本地目录。
- 修复：测试显式设置 `CatalogStale:true`，使其符合配置 API 的真实调用契约；随后仍要求成功同步后才切换到上游目录。
- 验证：本次修正尚未由 Actions 验证；不得据此宣称 Go test 通过。

## 2026-11-18 — 修正 PR #26 上游目录切换测试（配置变更后必须先同步）

- 状态：测试修正已提交，等待 Go CI 复验；PR 未合并。
- PR：https://github.com/seanldnet/TG-Card/pull/26
- Commit：`f46a33296ef83c8619abfb7be5edc8225a0e3a28`
- 根因：上一轮 Go test 仍按“仅保存 Enabled=true 即立即展示上游目录”的旧假设编写断言，与当前安全策略不符。配置变化会将目录标记为 stale，必须成功完成一次同步后才能切换到上游目录。
- 修复：测试先确认 stale 状态仍展示本地目录，再执行成功同步并确认切换到上游目录，随后验证成功的空目录同步会隐藏已不再存在的上游商品。
- 验证：修复后的 Actions 尚待完成；不把提交或测试断言当作通过证据。

## 2026-11-18 — 修复 Dujiao-Next 分类图标同步回归（PR #26）

- 状态：修复已提交，等待最新 PR CI 复验；未合并。
- PR：https://github.com/seanldnet/TG-Card/pull/26
- 修复 commit：`c402bc8345fb6d2cfc33350457e8083e87fe0948`
- 根因：分类图标更新 SQL 的 CASE 表达式占位符顺序与参数顺序不一致；当图片下载失败、要求保留旧图标时，旧逻辑把布尔值写入 `upstream_icon`，导致测试失败。
- 修复：按 SQL 占位符顺序传入 `PreserveIcon` 与新图标值；保留既有图标的分支不再写入布尔值。
- 验证：上一轮 Go vet 通过；Go test 的 `TestUpstreamCatalogSyncIsTransactionalAndSourceAware` 暴露此问题。修复后的 Go test/build 与完整 CI 尚待新一轮 Actions 结果确认。
- 资金安全：本修复不涉及订单、买家扣款、供应商采购或退款逻辑。

## 2026-11-18 — Dujiao-Next PR B：商品目录同步与来源管理

- 状态：PR #26 已合并至 main；Go CI、Admin CI、安全扫描及 Docker Publish 均成功；未执行真实商户账号同步验收。
- 分支：`feat/dujiao-catalog-sync`；开发前 main 基线 `e625d57473acba618cb26923965566cd3fea12d1`。
- 复现条件：PR A 已可保存 Dujiao-Next 凭证并测试连接，但没有商品/分类/SKU 同步；启用上游目录无法切换 Bot 商品来源。
- 变更内容：
  - 数据库迁移 0030–0034：分类/商品 source_type、上游分类/商品/SKU ID、SKU 规格、履约类型/人工表单 schema、标签、库存、原币价格快照、图片本地路径、同步批次、加价/汇率策略及目录 stale 标志。
  - 分类、商品、SKU 通过官方 API 分页同步；每个上游 SKU 映射为一条 TG-Card 商品，保留 product_id、sku_id、sku_code、spec_values、fulfillment_type 和 manual_form_schema。
  - 上游图片/分类图标下载至 DATA_DIR/uploads/upstream，内容校验、大小限制、SHA-256 文件名；不保存远程图片 URL 作为展示资源。
  - 价格保存原币快照；最终售价先按显式配置汇率换算为 USDT，再应用百分比加价与固定加价。上游币种为 USDT 时汇率固定 1；非 USDT 未配置汇率则拒绝同步，不猜测汇率。
  - 手动同步 API 与按配置间隔运行的定时同步；目录 upsert 在单个数据库事务内完成，上游下架/缺失 SKU 在成功同步后隐藏；网络/数据/事务失败不覆盖上次成功目录。
  - 配置、凭证或价格策略变更会标记目录 stale 并自动关闭上游展示；重新同步成功后才允许再次启用。
  - Bot 启用上游后仅展示上游商品；关闭后恢复本地商品。PR C 采购转发完成前，上游商品不展示可购买/确认按钮，后端也拒绝伪造订单或扣款。
  - 后台本地旧商品在上游启用时置灰保留；上游商品禁止本地编辑、删除和导入卡密。
- 数据库迁移：新增 0030–0034；兼容 SQLite/PostgreSQL。
- 配置/部署影响：无新增环境变量；汇率、加价和同步间隔在后台配置；图片使用现有 DATA_DIR 持久化卷。
- 兼容性与已知限制：当前订单结算为 USDT；非 USDT 上游币种必须人工配置“1 单位上游币种 = ? USDT”的汇率。每个 SKU 单独映射为商品；最多下载每商品前 5 张图片。上游采购/付款转发尚未实现，必须等待 PR C。
- 验证：
  - 新增官方响应 JSON 结构、价格解析/汇率/加价、SKU 规格名称单元测试。
  - 新增 SQLite 源切换、事务回滚、缺失商品隐藏、图片失败保留、stale 标志和设置持久化测试。
  - PR #26 Go CI、Admin CI、安全扫描均成功；真实上游账号同步待项目所有者执行。
  - 官方协议依据：`docs/upstream-integration.md` 所列 Dujiao-Next Open API 文档。
- 发布镜像：PR #26 已发布；Publish Run #44299965343 成功，digest `sha256:c805ce4a8aa2ee6f2adcf01d16122d15117c9e813457a740c00f2d31e7ec66ae`；后续 latest 已由 PR #27/#28 更新。

## 2026-11-18 — Dujiao-Next PR A：上游基础设施（配置与连接测试）

- 状态：PR #21 已合并至 main；Go CI、Admin CI、安全扫描均通过；真实商户账号验收待执行。
- PR / Commit：[PR #21](https://github.com/seanldnet/TG-Card/pull/21)；merge commit `2044fa5234ebfd62d3e30357220b192e3a10814b`；开发分支 `feat/dujiao-upstream-foundation`。
- 复现条件：后台没有上游配置入口；没有持久化商户凭证、连接测试或上游状态记录。
- 变更内容：
  - 新增数据库迁移 `0029_upstream_settings.sql` 和上游配置持久化方法，记录 provider、站点根地址、加密凭证、启用状态、连接测试结果/错误与同步状态字段。
  - API Key（商户 ID）与 API Secret 均使用现有持久化 AES-256-GCM 密钥加密；读取只返回掩码和已配置标志，不返回明文。
  - 新增 Dujiao-Next 官方 HMAC-SHA256 签名实现及 `POST /api/v1/upstream/ping` 连接测试；按官方规范计算空请求体 MD5、签名原文和请求头。
  - 上游站点仅接受 HTTPS 根地址；请求不跟随重定向，并在拨号阶段解析 DNS、拒绝私网/本地/非公网 IP，降低 SSRF 风险。
  - 新增后台“上游对接”页面与动态菜单，提供上游类型、站点地址、商户 API Key/Secret、启用开关、测试连接、连接结果和同步状态/错误展示。
  - 当前只开放 Dujiao-Next；ACG-Faka、MCY 不接受未核实的协议类型，不实现其 adapter。
- 数据库迁移：新增 0029；SQLite/PostgreSQL 共用迁移与 upsert。
- 配置/部署影响：无新增环境变量；使用现有 DATA_DIR 持久化加密密钥。
- 兼容性与已知限制：PR A 不执行目录同步、不改变本地商品展示、不采购、不扣上游余额；同步状态字段仅用于后续 PR B。真实商户凭证连接测试尚待测试环境执行。
- 验证：
  - 新增 Dujiao 签名规范与站点 URL 校验单元测试。
  - 新增 SQLite 上游配置读写、状态持久化测试。
  - Go CI：[Run #44291154727](https://github.com/seanldnet/TG-Card/actions/runs/44291154727) 成功；Admin CI：[Run #44291154731](https://github.com/seanldnet/TG-Card/actions/runs/44291154731) 成功；安全扫描：[Run #44291154797](https://github.com/seanldnet/TG-Card/actions/runs/44291154797) 成功。
  - 官方协议依据：[Dujiao-Next Open API](https://dujiao-next.com/api/integration-open-api) 与 [站点对接指南](https://dujiao-next.com/api/integration-guide)。
- 发布镜像：[Publish Run #44291422271](https://github.com/seanldnet/TG-Card/actions/runs/44291422271) 成功；GHCR latest 与 commit 标签均已推送；digest `sha256:edcf23eefc6be73bd1433a94d74870cae18d2198cd75424e7918d2b6f3066479`；构建平台 linux/amd64、linux/arm64。
- 后续：PR B（PR #26）已合并；下一步按 PR C→D→E 推进。异次元与萌次元继续保持不开发适配器的限制。

## 2026-11-18 — PR #19 镜像发布核验与追踪状态修正

- 状态：镜像已发布；真实环境验收待项目所有者执行。
- PR / Commit：[PR #19](https://github.com/seanldnet/TG-Card/pull/19)；main commit `40cb88ea3e3b9e204f8a48b8c59cf15c8ace05fb`。
- Publish Actions：[Run #327](https://github.com/seanldnet/TG-Card/actions/runs/44288929603)，Run ID `44288929603`，事件 push，状态 `completed / success`；job `build-and-push` 成功。
- 镜像标签：`ghcr.io/seanldnet/tg-card:latest`、`ghcr.io/seanldnet/tg-card:40cb88ea3e3b9e204f8a48b8c59cf15c8ace05fb`。
- 镜像 digest：`sha256:05663ddbc49d9706a4fe3b4771cbc1dab427a29b15605a048d69723485fc60c6`；Buildx 日志明确记录 latest manifest 推送成功、commit SHA 标签推送成功及 containerimage.digest。
- 后续 latest 关系：PR #21 的 `2044fa5234ebfd62d3e30357220b192e3a10814b` 相对 PR #19 的 `40cb88ea3e3b9e204f8a48b8c59cf15c8ace05fb` 为后代（ahead_by=2、behind_by=0）；当前 latest digest `sha256:edcf23eefc6be73bd1433a94d74870cae18d2198cd75424e7918d2b6f3066479` 因此包含 PR #19 修复。
- 构建平台：linux/amd64、linux/arm64。
- 同一 commit 的 Go backend CI、Admin frontend CI、安全扫描均成功；见 [Go](https://github.com/seanldnet/TG-Card/actions/runs/44288929665)、[Admin](https://github.com/seanldnet/TG-Card/actions/runs/44288929601)、[Security](https://github.com/seanldnet/TG-Card/actions/runs/44288929734)。
- 验收状态：Telegram 图片/文件附件及 Bot 停止后历史附件查看、跟踪防护下 Network 零外部 CDN 请求、PC 宽屏真实浏览器回归，均仍待项目所有者执行。CI 与镜像发布不等于端到端验收。
- 文档：更新 PROJECT_STATUS 元信息源码基线为 `main@40cb88ea3e3b9e204f8a48b8c59cf15c8ace05fb`；重写第 19 节 PR/发布/验收追踪表；修正本记录之前将 PR #19 标记为“PR 中”的过期状态。

## 2026-11-18 — 附件下载、静态资源与 PC 内容区修复

- 状态：已合并至 main（PR #19）；镜像已发布（Publish Run #327 成功，digest 见下方发布核验记录）；真实环境验收未完成。
- 复现条件：售后附件端点依赖内存 Bot worker，下载失败日志不足；Vben 默认 Logo/头像使用 unpkg static-source；后台外层容器 max-width 限制大屏宽度。
- 附件：`internal/telegrambot/manager.go` 从 `bots.token_ciphertext` 读取工单所属 Bot Token 并使用既有 decrypt 函数解密，不再依赖运行 worker；getFile 与下载链路分别设置超时；20 MiB 限制。
- 附件错误与日志：`cmd/server/support.go` 区分附件失效、文件过大、Telegram 异常、未知错误；结构化记录 ticket/message/bot、脱敏 file_id 前缀、Telegram 错误码/描述和成功字节数，不记录 Token。
- 前端提示：`web/overrides/apps/web-antd/src/views/support/index.vue` 根据 HTTP 410/413/502 显示不同文案。
- CDN：新增本地 `src/assets/tg-card-logo.svg`，覆盖 Vben preferences 的 `logo.source` 和 `app.defaultAvatar`。
- PC 布局：订单、工单、商品、仪表盘、支付、用户、站点/支付设置等页面移除外层固定最大宽度，保留移动端断点。
- 上游协议：更新 `docs/upstream-integration.md`。Dujiao-Next 按官方 Open API 核实 HMAC-SHA256、目录/SKU、ping 余额、采购、查单与回调；ACG-Faka/MCY 未找到可核验的商户采购 API，保持待上游确认。
- 文档：同步更新 `docs/PROJECT_STATUS.md`；保留问题五审计记录。
- CI：Go backend CI、Admin frontend CI、安全扫描通过。Actions: [Go](https://github.com/seanldnet/TG-Card/actions/runs/44287702230)、[Admin](https://github.com/seanldnet/TG-Card/actions/runs/44287701536)、[Security](https://github.com/seanldnet/TG-Card/actions/runs/44287701277)。
- 未验收：Telegram 真机图片/文件预览下载、Bot 停止后查看历史附件、浏览器跟踪防护 Network 零外部域请求、PC 宽屏真实浏览器回归、真实上游下单。以上由项目所有者执行，不由 Agent 代标已验收。

## 2026-11-18 — 上游发卡能力与文档一致性审计（问题五，PR #17 已合并）

- 状态：PR #17 已合并；审计结论已写入文档；代码功能未因此宣称交付。
- 源码基线：`main@41ef99ac935407dfd896b74840e220b3ba766400`。
- 检查范围：`README.md`、`docs/*`、`web/overrides/`、`internal/`、`cmd/`，并核对数据库迁移、后台路由/页面及动态菜单注册。
- 关键代码证据：
  - `internal/upstream/adapter.go` 只有 provider-neutral 数据结构、Provider 常量及 Adapter 接口定义。
  - `internal/upstream/adapter_test.go` 仅覆盖 Provider 标识和未知库存语义。
  - `internal/database/migrations/` 没有上游配置/商品来源迁移。
  - `cmd/server/main.go` 未注册上游配置、同步、测试连接或余额 API。
  - `web/overrides/apps/web-antd/src/views/` 没有上游对接页面；路由/动态菜单也没有上游入口。
  - 未发现 ACG-Faka、MCY Shop 或 Dujiao-Next 的可调用 provider adapter 实现。
- 审计结论表：

| 能力项 | 文档位置 | 代码证据 | 真实状态 |
|---|---|---|---|
| Dujiao-Next 目录/商品协议 | `docs/upstream-integration.md` 第 5 节 | 无 provider adapter、API 路由、配置表或后台页面 | 仅文档调研/规划，未实现 |
| Dujiao-Next 下单/查单/余额 | `docs/upstream-integration.md` 第 5 节 | 无网络请求、签名、订单转发或余额查询代码 | 仅文档调研/规划，未实现 |
| 异次元（ACG-Faka）目录/下单/查单 | `docs/upstream-integration.md` 第 3 节 | 无 ACG 请求/签名适配器与后台入口 | 仅文档调研/规划，未实现 |
| 萌次元（MCY Shop）外部商户 API | `docs/upstream-integration.md` 第 4 节 | 无 MCY adapter；文档明确官方外部商户协议待确认 | 仅源码线索与待确认事项，未实现 |
| 通用上游适配器契约 | `internal/upstream/adapter.go` | Provider 常量、数据类型、接口与 `ErrUnsupported` | 部分实现：仅接口契约，无 provider 网络实现及后台入口 |
| 上游配置、密钥管理、连接测试 | `docs/upstream-integration.md` 第 7 节（实施规划） | 无上游配置表/API/页面；无该模块密钥读写流程 | 纯规划，未实现 |
| 上游目录同步、图片本地化、来源过滤 | `docs/upstream-integration.md` 第 7 节（实施规划） | 无同步任务、来源字段迁移或 catalog service | 纯规划，未实现 |
| 上游采购转发、幂等、失败队列、退款/告警 | `docs/upstream-integration.md` 第 7 节（实施规划） | 无上游采购调用、订单状态/队列/余额告警实现 | 纯规划，未实现 |
| README 对上游集成的描述 | `README.md` | README 未宣称上游已交付；新增明确边界说明 | 已修正文档口径 |

- 结论：此前文档中的接口协议说明属于调研资料，不等于产品已集成。凡仅有通用契约而无 provider 网络实现/后台入口的能力，统一标注为“部分实现：仅接口契约，缺少 provider 实现与后台入口”；各供应商业务能力均不得标记为“已实现”。
- 文档更新：README 增加上游调研入口和未实现声明；PROJECT_STATUS 增加上游模块真实状态及未完成追踪；本记录补充审计证据表。
- 验证：基于 main 树结构、Go 源文件、迁移文件、后台页面/路由目录和项目文档静态核对；未执行代码测试（本次仅文档审计）。
- 后续：按指定顺序进入附件、CDN、PC 布局，再对有官方证据的供应商协议实施；MCY 未确认项继续保持不实现。
 
# TG-Card 开发与修复日志

## 2026-11-18 — 售后工单附件后台预览与下载（阶段一，P0）

- 状态：PR 待合并；Telegram 真机验收待执行
- PR / Commit：https://github.com/seanldnet/TG-Card/pull/15；分支 `fix/support-attachment-visualization`；源码基线 `main@e70879e6bdbb5b554b31fdb06adaf6b3a359b98f`
- 影响模块：`internal/database/support.go`、`internal/telegrambot/manager.go`、`cmd/server/support.go`、`cmd/server/main.go`、售后后台页面、售后文档
- 复现条件：用户在 Telegram Bot 售后工单中发送图片/文件；后台时间线只能看到 Telegram file_id，无法预览或下载。
- 根因：数据库与消息 API 已保存/返回 attachmentFileId、attachmentName，但前端仅显示 file_id 文本；没有管理员附件读取接口。
- 修复内容：
  - 新增数据库附件归属查询，要求 messageId 必须属于指定 ticketId，且该消息确实包含附件。
  - 新增管理员 Bearer 认证附件端点；服务端使用工单所属 Bot 调用 Telegram getFile 并下载，拒绝客户端任意 file_id。
  - 图片以内嵌方式预览，其他文件作为附件下载；限制 20 MiB，失败时返回明确错误。
  - 前端时间线展示图片预览和下载链接，并在页面卸载/切换工单时释放 Blob URL。
  - 更新 PROJECT_STATUS、CHANGELOG 和 support-tickets 专项文档。
- 数据库迁移：无；复用 0025_support_conversation_sla.sql 中的 support_ticket_messages 表。
- 安全说明：附件端点要求管理员 Bearer Token；数据库校验工单与消息归属；不将 DATA_DIR 作为附件静态目录公开。
- 验证：
  - Go CI、前端 CI、代码安全扫描：待 PR workflow 运行结果回填。
  - Telegram 真机：用户发图/文件后后台预览、下载及跨工单访问拒绝，待项目所有者验收。
  - Bot 离线/file_id 失效时的错误提示，需在测试环境验收。
- 发布镜像：待 PR 合并后的 Publish workflow 成功后记录 tag/digest。

## 2026-11-18 — 扩充项目状态文档：配置、升级、API、测试、运维与安全（文档 v1.1.0）

- 状态：已合并
- PR / Commit：https://github.com/seanldnet/TG-Card/pull/14；合并 commit `d77e46c197c121010c6a73a475e28a927130c25c`；分支 `docs/update-project-status-v1.1.0`；源码核对基线 `main@a06764f5060327ab675603bdf89115a11450951e`
- 影响模块：`docs/PROJECT_STATUS.md`、`docs/CHANGELOG.md`
- 变更内容：
  - 增加最后更新时间、源码 commit/分支和文档版本元信息。
  - 增加环境变量与配置清单，核对配置加载器、Compose、启动逻辑和密钥实现。
  - 增加升级/回滚说明，注明迁移事务行为、无 down migration 和旧版本 schema 兼容待验证。
  - 增加后台 API 模块索引与权限说明，区分公开、管理员 Bearer 和 super 管理员。
  - 增加 Go/前端构建命令及 Telegram、支付、移动端端到端验收 checklist。
  - 将 PR #12 首页 Inline Keyboard 问题抽到追踪表，并更新为已合并、镜像/真机验收待核验。
  - 增加日志、健康检查、TRC20 人工核对 SOP、FAQ 和安全基线。
  - 为原第 11 节全部未完成事项增加优先级和建议阶段，保留原条目文字。
  - 在第 2 节末尾增加 Mermaid 架构图。
- 数据库迁移：无，仅文档变更。
- 配置/部署影响：无。
- 兼容性与已知限制：旧镜像回滚兼容矩阵、备份恢复 SOP、充值过期到账人工处理等证据不足内容明确标注待补充。
- 验证：
  - 核对 config loader、Compose、API 路由、JWT、密钥管理、迁移代码、CI workflow 和现有专项文档。
  - 文档变更，不涉及 Go 或前端代码测试。
  - 运维步骤和验收 checklist 是操作模板，仍需实际部署人员执行并填写结果。
- 发布镜像：不涉及

> 本文件按时间倒序记录仓库内每一次有意义的开发、修复、重构、数据库变更、配置变更和发布。新记录追加在文件顶部。每个 PR 都必须更新本文件；不得只在聊天中汇报。

## 2026-11-18 — 建立项目功能总览与变更记录规范

- 状态：文档提交待合并
- PR / Commit：待创建
- 影响模块：项目文档
- 变更内容：
  - 新增 `docs/PROJECT_STATUS.md`，以当前仓库源码、后台页面、API、数据库迁移和现有专项文档为依据，汇总当前功能、技术架构、部署方式、已知限制及未完成事项。
  - 建立每次开发/修复必须更新功能总览、变更日志和相关专项文档的强制规范。
  - 在 README 中增加文档入口。
- 数据库迁移：无
- 配置/部署影响：无
- 兼容性与已知限制：本文档区分“已实现”“部分实现”“待验收”和“待开发”；自动化 CI 不代表 Telegram/支付/生产环境已验收。
- 验证：
  - 对照 main 分支 README、Go API 路由、Telegram Bot Manager、后台业务页面、数据库迁移及现有 docs 编写。
  - 未执行代码变更，因此无新增代码测试。
- 发布镜像：不涉及

## 2026-11-16 — 售后、订单恢复、后台交互与 Bot 菜单改进（PR #11）

- 状态：已合并；对应 main commit `e76bb8284c0ad925b56126b4f9ee31ef51439000`。Docker 镜像发布工作流当时曾在运行；应以 Actions 最终结论和 GHCR 实际标签为准。
- PR：https://github.com/seanldnet/TG-Card/pull/11
- 影响模块：售后、订单交付、后台工单、订单列表、优惠券/兑换码页面、Telegram Bot 菜单。
- 变更内容：
  - 为已交付订单引入独立重发租约路径；邀请链接不允许通过该路径重复发放。
  - 售后描述步骤支持直接提交图片/文件及 caption，并保存 Telegram 附件标识。
  - 后台工单“处理”弹窗改为原生 Ant Design Vue Modal，并增加状态保存和错误提示。
  - 订单列表增加分页，调整 TRC20 提示区域间距。
  - 优惠券和兑换码说明文字缩小并自动换行。
  - 增加 Telegram 常驻 Reply Keyboard 与主人管理入口/按钮流程。
- 数据库迁移：该 PR 的上述改动不要求新增迁移。
- 配置/部署影响：Bot 更新后需在 Telegram 中发送 `/start` 重新展示菜单。
- 兼容性与已知限制：
  - Telegram 消息发送没有通用幂等键；交付重试需要识别可能重复的消息。
  - 常驻 Reply Keyboard 与消息内 Inline Keyboard 是两种不同 UI；PR #11 后首页 Inline Keyboard 显示仍需修正，后续见 PR #12。
  - 主人菜单中部分流程仍要求按提示输入文本。
- 验证：
  - Go Backend CI、Admin Frontend CI、安全扫描在 PR #11 合并后通过。
  - CI 不代表真实 Telegram 客户端、售后媒体上传或移动浏览器验收。
- 发布镜像：请以对应 Docker Publish Actions 和 GHCR 标签核验为准。

## 2026-11-18 — 恢复 /start 与主菜单欢迎卡片下方的 Inline Keyboard（PR #12）

- 状态：已合并；merge commit `e70879e6bdbb5b554b31fdb06adaf6b3a359b98f`。镜像发布和 Telegram 真机验收状态需单独核验。
- PR：https://github.com/seanldnet/TG-Card/pull/12
- Commit：`8c52bb7f103382cf85e235aeec30787c04216a79`
- 影响模块：Telegram Bot 首页菜单。
- 变更内容：
  - 首页欢迎卡片使用 `welcomeKeyboard` 恢复消息下方的 Inline Keyboard。
  - 通过第二条提示消息发送 `mainReplyKeyboard`，保留输入框下方的常驻键盘；Telegram 每条消息只能设置一种 reply_markup，因此分开发送。
  - `/start` 与“🏠 主菜单”共用首页函数，两处一并修复。
- 数据库迁移：无
- 配置/部署影响：合并并发布后，用户需重新发送 `/start` 刷新 Telegram 菜单。
- 兼容性与已知限制：必须在真实 Telegram Android/iOS/桌面端检查欢迎卡片 Inline Keyboard 与输入框下方 Reply Keyboard 同时出现；代码提交本身不代表客户端验收。
- 验证：
  - PR 创建时 Go 测试和安全扫描检查已排队，最终结果待补录。
  - Telegram 端到端验收待完成。
- 发布镜像：`ghcr.io/seanldnet/tg-card@sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e`
