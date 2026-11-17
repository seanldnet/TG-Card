# TG-Card 开发与修复日志

## 2026-11-18 — 扩充项目状态文档：配置、升级、API、测试、运维与安全（文档 v1.1.0）

- 状态：文档更新待合并
- PR / Commit：待创建；源码核对基线 `main@a06764f5060327ab675603bdf89115a11450951e`
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
