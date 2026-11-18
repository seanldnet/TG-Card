## 2026-11-18 — 附件下载、静态资源与 PC 内容区修复（综合修复阶段）

- 状态：代码改动已提交至修复分支；CI/浏览器/Telegram 真机验收尚未完成。
- 复现条件：工单附件访问返回 500/502 且缺少有效日志；Vben 默认 Logo/头像请求 unpkg CDN；PC 页面容器宽度被 max-w-7xl 限制。
- 附件改动：`internal/telegrambot/manager.go` 从数据库 `bots.token_ciphertext` 读取工单所属 Bot 凭据并使用现有 decrypt 函数解密，不再依赖运行中的 worker；getFile 与下载分别设置超时；限制 20 MiB；区分附件失效、文件过大、Telegram 异常和未知错误。
- 附件日志：`cmd/server/support.go` 记录 ticket/message/bot、脱敏 file_id 前缀、Telegram error_code/description、成功字节数；不记录完整 Token 或密钥。
- 前端提示：`web/overrides/apps/web-antd/src/views/support/index.vue` 按 HTTP 410/413/502 展示不同文案。
- CDN 改动：新增本地 `src/assets/tg-card-logo.svg`，在 Vben preferences 覆盖 `logo.source` 和 `app.defaultAvatar`，替换上游默认的 unpkg URL。
- PC 布局：订单、工单、支付、用户等页面去除固定 max-w-7xl 内容宽度上限；移动端断点和窄屏样式保留。
- 上游调研：重写 `docs/upstream-integration.md`，Dujiao-Next 按官方 Open API 文档核对路径、HMAC-SHA256、目录、SKU 下单、余额和异步回调；ACG-Faka/MCY 未取得可核验商户 API，标记待上游确认，不实现猜测字段。
- 文档更新：同步更新 `docs/PROJECT_STATUS.md` 与本日志。
- 验证结果：静态代码修改已推送到分支；尚未运行 CI，未验证跟踪防护下 Network 零外部域请求，未执行真实 Telegram 图片/文件及 Bot 停止状态验收，未进行真实上游采购。
- 验收责任：以上真实环境验收由项目所有者执行；Agent 不将其标记为已验收。

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
