# 售后工单与附件

## 工单创建与状态

- 用户在对应 Bot 私聊中通过商城的“申请售后”交互提交工单。
- 工单必须关联当前 Telegram 用户在当前 Bot 下的真实订单；未付款订单不能提交。
- 每笔订单同时只允许一个未关闭工单；已解决/拒绝/关闭后允许再次提交。
- 管理后台可查看最近 500 条工单，更新状态、分配管理员、设置 SLA 并填写管理员回复。
- 管理员回复会通过创建该工单的 Bot 私聊通知用户；Bot 未运行时回复仍保存在工单中，后台会记录通知失败。
- 状态包括 open、in_progress、waiting_user、resolved、rejected、closed；关键变更记录管理员审计。

## 附件存储与读取

- Telegram 用户提交图片或文件时，Bot 处理器从 Telegram Update 读取 photo/document 的 `file_id` 和文件名。
- 当前实现把 `file_id`、附件名称、消息文本和时间写入 `support_ticket_messages` 表的 `attachment_file_id`、`attachment_name` 等字段；**不把原始附件下载落盘到 DATA_DIR**。
- 管理后台的 `GET /api/support/tickets/{id}/messages` 返回时间线和附件元数据，不直接返回文件内容。
- 管理员通过受 Bearer Token 保护的 `GET /api/support/tickets/{id}/attachments/{messageId}` 获取附件。服务端先验证 messageId 与 ticketId 的数据库关联，再使用该工单所属 Bot 的 Token 调用 Telegram `getFile` 并下载文件。
- 附件接口只允许读取目标工单消息中已保存的 file_id；不接受客户端任意传入 file_id。找不到附件返回 404；Bot 未运行、Telegram file_id 失效或下载失败时返回明确错误，不会把 DATA_DIR 作为静态目录公开。
- 图片按 image MIME 类型内嵌预览；其他文件以附件方式下载。下载大小上限为 20 MiB。
- Telegram 文件接口的可用性受 Bot Token、Bot 运行状态和 Telegram 文件保留/可访问状态影响；失败时应检查 Bot 运行状态和应用日志。

## 数据库依据

- 时间线结构由 `internal/database/migrations/0025_support_conversation_sla.sql` 创建。
- 消息读写和工单附件归属查询位于 `internal/database/support.go`。
- Bot 接收 Telegram 媒体位于 `internal/telegrambot/manager.go`。
- 管理员附件路由和 Telegram 下载代理位于 `cmd/server/support.go`；路由注册位于 `cmd/server/main.go`。
- 管理后台预览与下载位于 `web/overrides/apps/web-antd/src/views/support/index.vue`。

## 验收边界

- 自动化测试/CI 通过不等于 Telegram 真机验收。
- 项目所有者需使用真实 Telegram 账号分别提交图片、PDF/文档等文件，并在管理后台验证预览/下载、跨工单访问被拒绝、Bot 不在线或文件失效时显示明确错误。
