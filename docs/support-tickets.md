# 售后工单、Owner Bot 转发与交互流程

> 当前状态（2026-11-20）：PR #27 Owner Bot 流程与 PR #28 结单生命周期均已合并。PR #28 首次发布镜像历史 digest 为 `sha256:89cd1443e63282e171c026460c545fce73e5ad9514c4b6452abe8b6b01fd1f04`；后续统一验收基准镜像为 PR S 合并发布的 GHCR 多架构 latest digest `sha256:f9f1dd001bd90c743264d3d16655226626842805f6f8af108c27eb8ff1b639ac`；该镜像尚未据此记录生产部署或项目所有者验收。PR #28 的 B1–B6 已按项目所有者本次确认记录已验收（2026-11-19）；PR #27 的附件/媒体转发子项已由项目所有者实测确认通过（2026-11-20）；C1–C4 的 Owner 回复、继续沟通、结单幂等、超时/重启/多工单隔离仍待验收；PR #44 补强旧确认按钮与切换工单的隔离保护。

> 安全修复记录（2026-11-20）：Owner Bot 待处理工单列表必须按当前 Bot ID 过滤；此前查询未限定 bot_id，存在跨 Bot 工单暴露及错误 Bot 上下文处理风险。PR #40 已合并（merge commit `69218784601f7d75b668b8047379d931c9a87564`）；PR head Go/Admin/Security 与 main Go/Admin/Security/Publish 均成功，GHCR latest digest `sha256:0a0ca07e37b42b46b6c6bfb6cc6a2519348066fc6ae62bf98eb4c5b09134b2d7`。生产部署与真机验收仍待确认。

> Owner 回复可靠性修复（2026-11-20，PR #41 已合并）：Owner Bot 现在先发送 Telegram，明确成功后才持久化客服回复；发送失败仅记录 system 事件并保留 reply 会话。Telegram 超时/响应丢失仍不能保证 exactly-once。main merge commit `af53dad378bb8d1d89385246bd7d11a308e8b7e0`；main Go/Admin/Security/Publish 成功，Publish Run #44567512730；该轮 digest 未独立核实，生产验收待确认。

> 网页后台客服回复可靠性修复（2026-11-20，PR #42 已合并（merge commit `7fd658d1803d18442d9284098ddf8128cf22cc19`））：审查发现网页后台的 `POST /api/support/tickets/{id}/messages` 仍先写 admin 时间线再调用 Telegram。PR #42 已合并：网页后台文本回复改为 Telegram 明确成功后才保存客服回复；明确失败只写 system 失败事件，工单回复字段与客服时间线不变，并返回 502 让前端重试。后台页面也改为先提交回复、成功后再更新工单状态，避免发送失败时状态先行变更。Telegram 已确认发送但本地保存失败时会明确报错，不宣称未送达。

> Owner 确认回调会话隔离（2026-11-20，PR #44 已合并（merge commit `0c705ed99c76488aeafa273b019fb3262d19d398`））：确认按钮必须绑定当前 Bot、Owner、Telegram chat_id、工单 ID 与 confirm 阶段；回复会话也必须绑定点击“开始回复”时的 Telegram chat_id。Owner 切换工单后，旧工单的“是，结单/否，继续沟通”按钮不得再操作旧工单；会话过期、Owner 切换工单或从其他 Telegram 聊天发送消息时也拒绝执行。已新增迁移 `0040_support_owner_session_chat.sql`；升级时清理无法安全绑定 chat_id 的旧活动会话，Owner 重新点击“开始回复”即可。开始回复时必须确认工单状态更新成功后才开启会话。PR head Go/Admin/Security 全部成功；main Go/Admin/Security/Docker Publish 全部成功，Publish Run [#44573663703](https://github.com/seanldnet/TG-Card/actions/runs/44573663703)，GHCR latest digest `sha256:64cbbf218f0aff25d0eddf6e784929a134596f1ae7c85b13bec48ba89cf5d1fe`。生产部署与 Telegram 真机验收未确认。

> 验收记录（2026-11-20）：项目所有者明确确认售后附件/媒体转发测试已亲自完成且无异常，因此“用户图片/文件由对应 Bot 转发至 Owner 私聊并收到原始媒体”子项记为**所有者实测通过**。本次未提供镜像 digest、设备、订单/工单号或截图，不将其绑定到某一发布镜像。此确认不覆盖 C1–C4 的 Owner 回复、继续沟通、结单幂等、超时/重启/多工单隔离，也不覆盖补发/退款全分支；这些仍保持待验收。

> C1/C2 自动化回归补强（2026-11-20，PR #47 已合并）：新增“否，继续沟通”后恢复 reply 阶段、不点按钮连续发送两条文字，以及 Owner 图片/普通文件回复分别调用 `sendPhoto` / `sendDocument` 并在成功后写入时间线的测试。该批次不替代真实 Telegram 验收。PR #47 head Go/Admin/Security 全部成功；main Go/Admin/Security/Docker Publish 全部成功，Publish Run [#44575484792](https://github.com/seanldnet/TG-Card/actions/runs/44575484792)，GHCR latest digest `sha256:3b763b2410a222c62ed864685ac2cba06c86fbc3942c69aa429d9b702cb02368`。

> C3/C4 自动化回归补强（2026-11-20，PR #50 已合并）：新增 Manager 重建后的数据库会话恢复、过期会话不发送/不写时间线，以及切换到工单 B 后 A 的旧确认按钮不能操作 A 或覆盖 B 会话的测试。该批次不替代真实 Telegram 验收。 PR #50 head 与 main Go/Admin/Security/Docker Publish 全部成功，Publish Run [#44577264695](https://github.com/seanldnet/TG-Card/actions/runs/44577264695)，GHCR latest digest `sha256:eb0776f9dafc7d90aa1928a9f85eeeaf1488891c16ab843826aa0db9ab2a1a4e`。

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

## PR1：结单通知、用户确认与重开

- Owner 在工单详情点“结单”、Owner 回复后确认结单，以及后台将工单状态改为 `resolved`，统一进入同一个事务状态转换：仅活动状态 `open/in_progress/waiting_user` 可首次转为 `resolved`；重复结单不重复写审计，也不重复通知用户。
- 结单成功后，Bot 私聊用户发送工单号、处理结果和“如仍有问题可直接回复本消息继续沟通”，并附两个 Inline Keyboard：
  - **问题已解决**：仅工单所属 Telegram 用户可点击；在工单仍为 `resolved` 时记录用户确认时间（审计记录的 `created_at`）和系统时间线；重复点击幂等。
  - **仍有问题，继续沟通**：仅工单所属用户可点击；仅允许 `resolved` 状态重新转为 `in_progress`，写入用户重开审计与系统时间线，并向 Owner 发送用户和工单编号、重新开启状态通知，附进入工单按钮。
- 结单通知发送失败（例如用户拉黑 Bot）只记录日志；已完成的数据库状态变更不回滚。通知失败不得伪装成发送成功。
- 用户后续消息转发前必须重新读取工单状态；只有 `open/in_progress/waiting_user` 允许保存为补充消息并转发给 Owner。`resolved/closed/rejected` 一律禁止继续转发至旧工单。
- 用户在没有进行其他明确输入流程时，向 Bot 发送新文字/图片/文件且最近工单为终态时，自动使用原用户、Bot、订单关联创建新工单，通知 Owner，并告知用户新工单号；旧工单追加系统时间线说明。该检查从数据库读取该 Bot/用户最近工单，不依赖进程内 pending action；Bot 重启后，最近的活动工单可继续接收补充消息，最近的终态工单仍会触发新工单创建。若原订单不再满足售后资格、已有其他活动工单或创建失败，向用户返回明确错误。
- 同一 Telegram 用户 10 分钟内最多自动新建一个后续工单，节流按用户全局持久化（跨 Bot），限流时提示稍后再试。限流不影响已存在活动工单的正常补充沟通。
- 后台结单和 Bot 结单共用 `database.ResolveSupportTicket`，确保状态、审计和系统时间线语义一致。

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

- 当前激活工单按 `(bot_id, owner_id)` 唯一保存，并绑定 `chat_id`；仅在启动回复会话的同一 Telegram 聊天中接收消息。新工单通知不会抢占当前会话。切换工单必须由 Owner 显式点击另一工单的“开始回复”。
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
- 新增迁移 `0035_support_owner_workflow.sql`：增加 Telegram update idempotency 字段及唯一索引、附件去重唯一索引、持久化 Owner 会话表、结单审计表。PR1 另新增 `0036_support_resolution_lifecycle.sql`：用户维度后续工单节流表及用户确认审计幂等索引。
- Owner 会话表以 Bot + Owner 为主键，保存 `chat_id`、ticket ID、阶段、过期时间和更新时间；会话查询同时校验 Bot、Owner 与 chat_id。迁移 `0040_support_owner_session_chat.sql` 为旧表增加 chat_id，并清理无法安全关联聊天的旧活动会话。
- 结单审计仅在工单从活动状态成功转为已解决时写入；重复结单不会产生第二条审计。
- SQLite 与 PostgreSQL 使用相同迁移语句及数据库层 placeholder 适配。
- 无新增环境变量；Owner ID 使用 Bot 管理中已有的 Owner ID，SMTP/Telegram 通知继续使用站点设置。
- PR1 回滚注意：先备份数据库并停止新版本服务，再回滚应用代码；若必须回滚迁移，仅删除 `support_followup_throttles` 表及 `idx_support_ticket_audits_action_actor` 索引。不得删除 `support_ticket_audits`、工单消息或历史工单；保留历史状态与审计数据。

## 代码入口

- Bot 更新、售后提交流程、Owner Inline Keyboard 与媒体发送：`internal/telegrambot/manager.go`
- 工单消息与附件历史数据：`internal/database/support.go`
- Owner 会话、幂等检查、结单事务和审计：`internal/database/support_owner.go`
- 新数据库结构：`internal/database/migrations/0035_support_owner_workflow.sql`
- 站点邮件/Telegram 通知：`cmd/server/main.go`
- 网站文字时间线：`web/overrides/apps/web-antd/src/views/support/index.vue`

## Owner 媒体回复的单次副作用限制

Owner 通过媒体回复用户时，caption（包含“客服回复：”前缀）最多允许 900 个 Unicode 字符。超过限制会在发送媒体前拒绝，并提示 Owner 缩短内容后重新发送。媒体与剩余文字不再拆成两次 Telegram API 调用，避免第一步已送达、第二步失败时造成时间线缺失及重试重复发送媒体。该限制只适用于 Owner 媒体回复，不限制普通文本消息。

## 验收边界

- 代码提交和 CI 通过不代表 Telegram 真机验收完成。
- 必须使用真实 Owner 与普通用户账号执行：图片+文字提交、文件转发、查看详情、开始回复、Owner 回复文字/媒体、继续沟通、不点击按钮连续发送三条消息、双工单切换、重复结单、会话超时、用户拉黑 Bot 失败、Owner 未配置降级、SMTP 邮件及站点 Telegram 通知。
- 上游采购/转发流程与本售后改造无关，禁止在本专项分支中混入采购逻辑。

## PR1 真实环境验收步骤（由项目所有者执行）

1. 使用普通用户提交工单，Owner 从 Bot 结单；确认用户立即收到工单号、处理结果和两个按钮。
2. 点击“问题已解决”；检查工单保持 `resolved`，时间线出现用户确认，审计只新增一条确认记录；重复点击不得重复写入。
3. 对另一张已结单工单点击“仍有问题，继续沟通”；确认状态变为 `in_progress`，Owner 收到重新开启通知和进入工单按钮。
4. 不点击按钮，直接向 Bot 发送文字、图片和文件；确认旧工单不再接收消息，Bot 自动创建关联原订单的新工单并告知新工单号。
5. 在 10 分钟内再次触发终态工单自动新建；确认提示稍后再试。10 分钟后重试应允许创建。
6. 在后台将活动工单结单；确认用户收到与 Owner Bot 结单相同格式的通知。
7. 使用拉黑 Bot 的用户执行结单；确认数据库状态保持已解决，服务日志记录发送失败，不重复结单或回滚状态。


## 全局异常中心联动

- 售后退款因活动再次获取租约而返回 HTTP 409 时，登记 `refund_conflict` 异常；来源键按工单稳定去重。
- 全局异常解决状态不代表退款已完成，不会自动退款、改变钱包余额或重试交付。必须回到工单/订单原流程核实后再填写处理结论。
- 详见 `docs/system-exceptions.md`。