## 2026-11-18 — Dujiao-Next PR B：商品目录同步与来源管理

- 状态：开发分支已实现，PR/CI/合并待完成；未执行真实商户账号同步验收。
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
  - Go CI、Admin CI、安全扫描等待 PR workflow 结果；真实上游账号同步待项目所有者执行。
  - 官方协议依据：`docs/upstream-integration.md` 所列 Dujiao-Next Open API 文档。
- 发布镜像：PR B 尚未合并，不更新 latest。

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
- 后续：PR A CI 通过并合并后再开始 PR B；异次元与萌次元继续保持待上游确认。

## 2026-11-18 — PR #19 镜像发布核验与追踪状态修正

- 状态：镜像已发布；真实环境验收待项目所有者执行。
- PR / Commit：[PR #19](https://github.com/seanldnet/TG-Card/pull/19)；main commit `40cb88ea3e3b9e204f8a48b8c59cf15c8ace05fb`。
- Publish Actions：[Run #327](https://github.com/seanldnet/TG-Card/actions/runs/44288929603)，Run ID `44288929603`，事件 push，状态 `completed / success`；job `build-and-push` 成功。
- 镜像标签：`ghcr.io/seanldnet/tg-card:latest`、`ghcr.io/seanldnet/tg-card:40cb88ea3e3b9e204f8a48b8c59cf15c8ace05fb`。
- 镜像 digest：`sha256:05663ddbc49d9706a4fe3b4771cbc1dab427a29b15605a048d69723485fc60c6`；Buildx 日志明确记录 latest manifest 推送成功、commit SHA 标签推送成功及 containerimage.digest。
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
- 发布镜像：待发布
