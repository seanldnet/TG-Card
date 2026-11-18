# 售后工单、Owner Bot 转发与交互流程

## 产品规则

- 工单仍关联真实订单、提交用户和创建工单的 Bot；同一订单同时只允许一个未关闭工单。
- 工单状态：`open`、`in_progress`、`waiting_user`、`resolved`、`rejected`、`closed`。
- 网站后台只展示文字时间线。媒体消息显示 `[图片]` 或 `[文件]` 占位，并提示“附件请通过机器人查看”。
- 原始 Telegram `file_id`、附件名和历史消息字段继续保留在 `support_ticket_messages`；旧附件读取 API 与数据库字段不删除，以兼容历史数据，但后台前端不再调用附件读取端点。该读取方式已由 Bot 转发 Owner 模式替代。

## 用户提交与 Owner 转发

- 用户通过商城“申请售后”入口选择关联订单，提交文字、图片或文件。
- Bot 将用户消息写入 `support_ticket_messages`。图片/文件继续保存 Telegram file_id 与文件名；原始媒体不下载到 DATA_DIR。
- 工单创建后，创建工单的 Bot 私聊该 Bot 配置的 Owner ID，发送工单号、用户 ID、Bot 名称、订单号和文字摘要，并附“查看工单”和“待处理列表”Inline Keyboard。
- 用户后续补充的文字/图片/文件也会转发给 Owner，并保留在同一工单时间线。
- 媒体使用同一 Bot 可访问的 Telegram file_id 重新发送给 Owner；Owner 收到的是原始图片/文件内容，不是网页下载链接。
- 如果该 Bot 未配置 Owner ID，Bot 不尝试发送媒体或通知；工单与消息仍正常存档，日志记录 `reason=owner_not_configured`。
- Owner 转发失败不回滚工单创建或用户消息存档；错误写入应用日志。站点已有 `support_created` 事件继续驱动 Telegram 通知与 SMTP 邮件通知。邮件收件人为站点联系人邮箱；SMTP 或联系人邮箱未配置时跳过并记录日志。通知失败不影响工单主流程。

## Owner Inline Keyboard 操作

### 待处理列表

- 新工单通知提供“查看工单 #工单号”和“待处理列表”按钮。
- 列表仅包含 `open`、`in_progress`、`waiting_user` 工单，每页最多 8 条。
- 每条工单一个按钮，显示工单号、用户标识及问题摘要；超过 8 条提供上一页/下一页。
- Owner 可从 Reply Keyboard 常驻入口“待处理工单”进入列表；“返回”回到 Owner 管理首页。

### 工单详情

详情展示工单号、用户 ID、Bot 名称、关联订单、状态、文字时间线及媒体统计占位，例如：

`[图片 2 张 / 文件 1 个，已在上面转发]`

详情按钮：
- **开始回复**：建立该 Owner 在该 Bot 下的当前激活工单。
- **结单**：将工单设为已解决，并写入 Owner 操作审计。
- **返回列表**：返回待处理列表。

### 回复状态机

Owner 点击“开始回复”后，Bot 提示直接发送文字/图片/文件。无需斜杠命令。会话保存在数据库，默认 30 分钟无操作失效。

1. 空闲 → Owner 点击“开始回复” → 已打开工单。
2. Owner 发送文字/图片/文件 → Bot 先尝试发送给工单用户；发送成功后写入工单消息时间线。
3. 发送成功后进入“已发送待确认结单”，Bot 显示“已发送给用户。是否结单？”并提供“是，结单”和“否，继续沟通”按钮。
4. 点击“是，结单”：工单原子更新为 `resolved`，记录 Owner ID、操作与时间；清除激活会话。重复点击不重复写审计，旧按钮对已结单工单仅提示“该工单已结单”。
5. 点击“否，继续沟通”：会话回到已打开工单，Owner 可继续直接发消息。
6. 不点击按钮而直接再发消息：仍按当前激活工单发送，成功后再次弹出结单确认，不会自动关闭工单。

### 并发、超时与失败边界

- 当前激活工单按 `(bot_id, owner_id)` 唯一保存；新工单通知不会抢占当前会话。切换工单必须由 Owner 显式点击另一工单的“开始回复”。
- 激活会话默认 30 分钟过期；过期后消息不会发给旧工单用户，Owner 需重新从列表打开工单。
- Owner 回复发送失败（例如用户拉黑 Bot、用户不存在或 Telegram API 错误）时，Bot 向 Owner 返回 Telegram 错误描述，工单不结单，失败消息不会标记为已送达。
- Telegram update/message ID 用作时间线写入幂等键，重复处理同一更新不重复插入消息；Owner 回复在发送前检查该键，避免同一更新再次发送。
- 多工单并发以数据库当前激活会话为准，不使用内存变量作为唯一状态来源。
- 不使用 `/reply` 等命令式售后入口。

## 网站后台展示

- 工单消息 API 仍返回附件元数据，兼容旧数据和既有客户端。
- 当前后台前端只渲染消息文字；有附件时显示 `[图片]` / `[文件]` 占位及“附件请通过机器人查看”。
- 后台不再调用 `GET /api/support/tickets/{id}/attachments/{messageId}`；该端点保留用于兼容，不删除。
- 工单详情页明确提示媒体凭证需在对应 Bot 私聊中查看。
- 网站文字回复和工单状态管理仍保留；Owner 交互式 Bot 流程是媒体与日常即时沟通的主要路径。

## 数据库与兼容

- 历史消息表仍保留 `attachment_file_id`、`attachment_name` 字段。
- 新增迁移 `0035_support_owner_workflow.sql`：增加 Telegram update idempotency 字段及唯一索引、附件去重唯一索引、持久化 Owner 会话表、结单审计表。
- Owner 会话表以 Bot + Owner 为主键，保存 ticket ID、阶段、过期时间和更新时间。
- 结单审计仅在工单从活动状态成功转为已解决时写入；重复结单不会产生第二条审计。
- SQLite 与 PostgreSQL 使用相同迁移语句及数据库层 placeholder 适配。
- 无新增环境变量；Owner ID 使用 Bot 管理中已有的 Owner ID，SMTP/Telegram 通知继续使用站点设置。

## 代码入口

- Bot 更新、售后提交流程、Owner Inline Keyboard 与媒体发送：`internal/telegrambot/manager.go`
- 工单消息与附件历史数据：`internal/database/support.go`
- Owner 会话、幂等检查、结单事务和审计：`internal/database/support_owner.go`
- 新数据库结构：`internal/database/migrations/0035_support_owner_workflow.sql`
- 站点邮件/Telegram 通知：`cmd/server/main.go`
- 网站文字时间线：`web/overrides/apps/web-antd/src/views/support/index.vue`

## 验收边界

- 代码提交和 CI 通过不代表 Telegram 真机验收完成。
- 必须使用真实 Owner 与普通用户账号执行：图片+文字提交、文件转发、查看详情、开始回复、Owner 回复文字/媒体、继续沟通、不点击按钮连续发送三条消息、双工单切换、重复结单、会话超时、用户拉黑 Bot 失败、Owner 未配置降级、SMTP 邮件及站点 Telegram 通知。
- 上游采购/转发流程与本售后改造无关，禁止在本专项分支中混入采购逻辑。
