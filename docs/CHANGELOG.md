## 2026-11-23 — Redis 共享限流与 Bot 轮询租约（开发中）

- 新增依赖零引入的 Redis RESP 客户端，支持 redis/rediss、TLS 1.2+、密码/ACL 认证与 SELECT 数据库。
- 新增原子 fixed-window 共享限流；管理员敏感操作限流和登录失败计数在多实例间共享。Redis 已配置但不可用时敏感请求 fail closed；未配置时沿用本地内存策略。
- Telegram Bot polling worker 使用 Bot ID 级 Redis lease（随机 ownership token、SET NX PX、token 校验续租/释放），持有租约实例负责 getUpdates；其他实例 standby，租约续租失败则取消轮询并重试。
- 订单交付继续沿用数据库持久化租约与既有幂等流程，不用 Redis 替换资金/履约安全基线。
- 增加 detailed health Redis 状态、Compose opt-in Redis profile 与环境变量说明；无数据库迁移。
- 验证：新增 Redis 客户端/协议/租约脚本单测；Go/Admin/Security CI 待 PR 执行，多实例和真实 Telegram 行为待部署验收。
- 状态：开发分支 `feat/redis-shared-rate-limiting`，尚未合并或发布。

---

## 2026-11-23 — Telegram Bot 与 Admin 中英文国际化交付（文档版本 2.2.99）

- Telegram 用户端 PR #145–#149 已合并，覆盖语言识别/偏好持久化、欢迎页/菜单、商品与订单详情、购物车/结账、支付执行、交付与售后通知。
- PR #164 增加每 Bot 默认语言设置（简体中文/英文），存入现有 Bot menu_config JSON，无数据库迁移；语言优先级为用户已保存偏好、Telegram 支持语言、Bot 默认语言；旧 Bot 默认简体中文。
- PR #164 本地化 Owner 菜单与经营概览；PR #166 本地化 Owner 售后工单列表、详情、回复、结单和通知；PR #169 本地化 Owner 订单、库存、用户、群发、封禁/解封和待付款订单管理操作。
- Admin Vue i18n PR #150–#155、#160 已合并，覆盖核心运营页面、经营分析、支付设置、管理员 TOTP、网站/备份/邮件/Telegram 通知设置及个人资料/密码。
- 当前 main commit：edb2287aeb8429435b9a3474c32212a746906e6a。PR #169 合并后的 Go Backend CI、Admin Frontend CI、安全扫描及 Docker Publish 均成功；GHCR 已推送 `latest` 与 commit tag，OCI manifest digest：`sha256:38d0a4feddb11c55a301db8d4c22a64878f5f5952e189a75375cc78597b937d7`。
- 本轮不改变支付、订单、库存或权限业务逻辑；无数据库 schema 迁移。PostgreSQL（pgx）仍为主数据库，SQLite 兼容。
- 生产部署及真实 Telegram Bot 端到端验收不由代码合并自动推定。

---

## 2026-11-23 — Telegram 与 Admin i18n 进度校准（文档版本 2.2.98）

- Telegram Bot 中英文 i18n：PR #145–#149 已合并，依次覆盖基础语言识别/偏好持久化、商品与订单详情、购物车/结账、支付执行、交付和售后通知。
- Admin Vue i18n：PR #150–#155 已合并，覆盖核心运营页面、支付/优惠券/广播/对账、商品目录/库存/全局异常、供应商配置/采购页面、资金异常、兑换码管理及运营工作台。
- PR #155 已通过 Admin Frontend CI 并合并至 main，合并提交 b0949dfaab0acb33cb12834cc836944e5cfbf773。
- 后续国际化重点为核对 Admin 仪表盘、系统设置及剩余页面，并检查 Telegram Owner 工作流、广播及每 Bot 默认语言配置。
- 本次仅校准开发状态与路线图，不改变业务逻辑、API、数据库 schema 或部署配置。PostgreSQL（pgx）仍为主数据库，SQLite 兼容。
- 生产部署、移动端真实设备及真实 Telegram Bot 端到端验收不由代码合并自动推定。

---

## 2026-11-23 — PR #143 Telegram Bot i18n 第一阶段验证与修复（进行中）

- PR #143 的 Go Backend CI（全量 go test、go vet、Docker 备份恢复集成测试、Go 构建）、Admin Frontend CI 与 Security Scan 均已成功。
- 复核确认项目使用 PostgreSQL（pgx）及 SQLite；移除误加的 MySQL 专属 RowsAffected 兼容查询，保留同语言重复选择的幂等测试。
- 当前第一阶段仍未覆盖商品详情、购物车/结账/付款、发货与售后通知、Owner 工作流等完整业务流程；常驻 Reply Keyboard 等残留文案仍需继续补齐。
- PR #143 尚未合并；CI 通过不代表生产部署或真实 Telegram Bot 端到端验收。

## 2026-11-23 — PR #140/#142 已合并，P1-3 Bot i18n 推进中（文档版本 2.2.96）

- PR #140 AI 售后基础架构已合并；包含通用 Chat Completions 调用层、加密模型配置、受保护管理 API、敏感意图转人工、订单事实型上下文、运营 Prompt、独立 AI 工单状态及后台标记/筛选。
- PR #142 第一批多模型适配已合并至 main：OpenAI、DeepSeek、Kimi 国际/中国大陆端点、智谱 GLM provider registry；包含 token-limit 参数选择、错误分类和 mock 契约测试。
- PR #142 合并后 main commit：`44fb370850c97c0c85dc1bddd2326fd38ae28a55`。合并状态不代表真实供应商 API 联调、生产部署或生产验收。
- 尚待完成：DeepSeek thinking 后台配置、完整响应/finish-reason 归一化及真实 API 联调。
- PR #143 正在推进 Telegram Bot 中英文 i18n 第一阶段，当前 PR 仍为 Open；未覆盖的商品详情、购物车/支付、发货与售后通知、Owner 工作流和每 Bot 默认语言列为后续范围。
- 同步校准 PROJECT_STATUS、ROADMAP 与 LLM_VENDOR_ADAPTERS；文档版本 2.2.96。

## 2026-11-23 — PR #140 AI 售后基础批次校准（文档版本 2.2.95）

- 已实现通用 OpenAI-compatible Chat Completions 调用层、模型配置加密存储与受保护管理 API、敏感意图拦截、有界上下文、订单归属校验后的事实型上下文、运营 Prompt，以及 AI 独立工单状态持久化和后台标记/筛选。
- AI 状态表不改变原工单生命周期；成功答复、转人工及内部原因码独立记录。附件仍转人工；AI 不执行退款、改价、补发、余额变更或其他资金/履约操作。
- PR head `4d1586cd90368c2069ed060cf6939615f1075678` 的 Go Backend CI（run 45051388701）、Admin Frontend CI（run 45051388699）、Security Scan（run 45051388725）均成功。
- PR #140 仍未合并、未发布镜像；上述 CI 成功不等于真实供应商联调、完整 Telegram 集成验收或生产部署完成。
- 文档校准：PROJECT_STATUS、ROADMAP、CHANGELOG 统一到 2.2.95；P1-1 标记为实现完成待 PR 合并，P1-2 调整为供应商专项适配与契约测试。


## 2026-11-22 — P1-1 AI 售后工单接入与安全兜底（历史阶段记录，已由 PR #140 后续实现覆盖）

## 2026-11-22 — AI 售后上下文与运营 Prompt（PR #140 开发中；文档版本 2.2.93）

- AI 售后模型上下文增加严格归属校验后的订单商品、状态、数量、金额币种及付款/发货时间；不传支付引用、凭据或交付 payload。
- 新增运营者可配置售后 Prompt（最多 3000 字），持久化于 settings 表，安全系统规则不可被覆盖。
- 管理后台 AI 售后客服配置页增加 Prompt 编辑区；图片/文件仍转人工。
- 新增 Prompt 存储边界与上下文构建测试。PR 未合并，CI 与发布状态以 GitHub 为准。


- 在通用 LLM Provider 请求层基础上，新增 AI 模型配置存储与后台 API：供应商、Base URL、模型、超时和启用状态保存于现有 settings 表。
- API Key 通过现有 AES-GCM Secret Manager 加密保存；后台读取仅返回配置状态，不回传密钥；更新配置受管理员二次确认和频率限制保护，并写入不含密钥的审计记录。
- 新增配置存储测试，覆盖密钥不以明文落库、加密往返及更新其他配置时保留原 API Key。
- 新增售后安全策略：退款、拒付、支付争议、隐私/账号安全、投诉、法律风险及明确要求人工的消息默认拦截，不进入模型调用；常规问题才允许进入 AI 路径。新增上下文构建器，限制最近 12 条有效对话、每条最多 2000 字符，排除系统事件和附件标识，并明确将历史文本视为不可信数据。
- 将 AI 接入新建工单首条纯文本描述及现有工单后续文本消息：仅 AI 已启用、工单未分配、最近一轮未由管理员回复接管且消息不含附件时调用；历史 AI 回复会以 assistant 角色进入后续上下文。成功答复写入工单时间线，标记为 system 类型“AI 售后预处理”。敏感意图、配置读取失败和模型/发送异常均向买家提示转人工，并保留现有 Owner Bot 工单通知路径。
- 上述条目是早期阶段快照；订单事实型上下文、可配置 Prompt、AI 独立状态字段及后台标记/筛选已由 PR #140 后续提交补齐。真实供应商联调及更完整 Telegram 集成测试仍待完成，PR 未合并/发布，不代表生产可用。

## 2026-11-22 — P1/P2 产品路线图纳入规划

- 新增 `docs/ROADMAP.md`，记录 AI 售后客服、多语言体系、上游 Provider Adapter、Redis 分布式协调及 S3/R2 对象存储的范围、实施顺序和验收标准。
- 项目所有者已确认按批次实施：P1-1 AI 售后基础架构起步，其后依次完成模型适配、Bot i18n、后台 i18n、上游 Adapter；再推进 Redis 与对象存储。
- 保持单站点、单运营者边界；Redis 是可选的多实例协调组件，不意味着多用户/多租户。
- 本条仅为路线图规划记录，未声称上述新功能已开发或发布。

## 2026-11-22 — 管理后台移动端交互与布局修复（PR #137 已合并、发布并验收）

- 修复备份加密密钥独立保存的二次确认白名单遗漏；此前前端请求二次确认时，后端因 actionKey 未登记而返回“该操作不支持二次确认”。
- 移除 TRC20 顶部独立启用区，将开关移动到 TRC20 收款配置卡片右上角，与 Epusdt/GMPay 网关卡片保持一致；保存按钮留在对应卡片内。
- 对账、资金异常和全局异常页面统一操作按钮尺寸；全局异常筛选表单改为纵向标签布局，增加字段与容器间距。
- 备份密钥生成/保存按钮与安全提示框增加间距，说明文案增加行高。
- 无数据库 schema 迁移。PR #137 已合并；Go Backend CI、Admin Frontend CI、Security Scan 与 Docker Publish 均成功。
- GHCR `latest` 与 commit tag `76a1a45fa23c51ceeb5d2f030a3f2798d8c76a24` 已推送；OCI manifest digest：`sha256:e60c1433e66f613cab06f2a7008679e9117ceaa61cd724359b0f0b7d1124e36e`。
- 项目所有者于 2026-11-22 确认生产部署后测试全部正常，包含二次确认、按钮尺寸、筛选间距、TRC20 卡片开关和备份密钥区域间距。

## 2026-11-22 — 网站设置管理数据库备份加密密钥（已由 PR #136 合并并发布）

- 网站设置 → 自动备份新增 32 位安全随机密钥生成、密码输入框与独立保存按钮。
- 密钥至少 32 个非空白字符，服务端通过 AES-GCM 加密保存；API 只返回配置状态，不回传密钥明文或密文。
- 每日自动备份优先使用网站设置密钥；未配置时回退兼容 BACKUP_ENCRYPTION_PASSPHRASE 环境变量。已配置密钥解密失败时直接失败关闭。
- 更换密钥不影响已生成的历史备份，但恢复历史备份必须使用当时的旧密钥。必须将当前密钥另存到密码管理器或离线安全位置。
- 手动命令行备份/恢复脚本仍从执行环境读取 BACKUP_ENCRYPTION_PASSPHRASE；本功能不改变脚本密钥来源。无数据库 schema 迁移。
- 新增每日备份密钥解析测试，覆盖网站设置优先、环境变量兼容回退、缺少密钥失败及密文损坏失败关闭。

## 2026-11-22 — 支付配置与后台交互体验优化（PR #135 已合并）

- 本轮范围：Epusdt/GMPay 支付配置简化、余额差异明细交互、对账月份选择器、后台按钮尺寸统一、中文提示修正。
- Epusdt 与 GMPay Edge 配置不再要求商家填写回调地址；服务端根据请求的公网来源自动生成对应回调路径。仅信任已配置可信代理的转发协议/域名头；启用网关时要求 HTTPS。
- 两个网关各自在配置卡片内提供启用开关和独立保存按钮；保留 TRC20 独立保存，不再让用户依赖页面顶部统一启用开关。
- 余额流水一致性检查的差异明细改为带加载状态的独立弹窗；初次加载历史记录不自动弹窗。对账月份选择改用 Ant Design Vue 月份选择器。
- 后台 Ant Design Vue 按钮统一尺寸；登录用户名/密码错误提示改为中文；资金检查结果、异常来源及关联对象的常见状态改为中文展示。
- 新增回调公网地址派生与可信代理头处理测试；无数据库迁移。
- PR #135 已 Squash Merge 至 main，合并提交 32cbf6b24c39f94d6df8a27408e3e16155670e3；合并后 Go Backend CI（run 44963456837）、Admin Frontend CI（run 44963456814）、Security Scan（run 44963456855）与 Docker Publish（run 44963456816）均成功。GHCR latest 与 commit tag 已推送，OCI manifest digest：sha256:2ce38874a8995c0f3bcb00e82ef6042f5e74dd547502030be0cc1af9ac489482。生产反向代理 HTTPS/可信代理配置仍需目标环境验证。

## 2026-11-22 — 本地与每日备份 age 加密（PR #132，已合并并发布）

- 状态：PR #132 已 Squash Merge 至 main；合并提交 `0f7c3da3145adb0e10ad37f8a362573032e3e081`。合并后 Go Backend CI、Admin Frontend CI、Security Scan、Docker Publish 均成功。
- PR / Commit：PR #132；分支 `feat/encrypted-backup-restore`；合并 commit `0f7c3da3145adb0e10ad37f8a362573032e3e081`。
- 影响模块：本地 PostgreSQL/应用数据备份、每日 Telegram Owner 备份、恢复脚本与 Docker 镜像。
- 变更内容：
  - 使用 age scrypt 流式加密本地数据库与应用数据归档、每日 Telegram 备份；加密后删除明文中间文件。
  - 增加镜像内 `backup-crypt` 工具；恢复前校验并解密到私有临时目录，之后才执行破坏性恢复。
  - 兼容恢复旧版明文备份；隔离测试增加密文与旧备份恢复覆盖。
- 数据库迁移：无。
- 配置/部署影响：新增 `BACKUP_ENCRYPTION_PASSPHRASE`，至少 32 个非空白字符；必须外部保管并在部署主机 .env 配置。缺失/过短时拒绝新建明文备份。
- 兼容性与已知限制：旧备份可恢复；丢失加密口令则新备份不可恢复。生产恢复演练尚未执行。
- 验证：
  - 自动化测试：PR 合并前 Go、Admin、Security 检查通过；隔离 Docker Compose 加密/解密恢复测试已通过。
  - 合并后 main：Go Backend CI、Admin Frontend CI、Security Scan、Docker Publish 均成功。
  - GHCR 标签：`ghcr.io/seanldnet/tg-card:latest` 与 `ghcr.io/seanldnet/tg-card:0f7c3da3145adb0e10ad37f8a362573032e3e081`。
  - OCI manifest-list digest：`sha256:fea9f69f008019fa08fa58cb551fa3fdae7cf836e18aa17df7c87c2fdbe0b9f8`。
- 发布镜像：已成功推送；生产部署与恢复演练仍待目标环境验收。

## 2026-11-22 — 支付查单资金核验修复（PR #131）

- 状态：已合并；PR head Go CI、Admin CI、Security Scan 全部通过；合并后 main CI 与 Docker Publish 已触发，最终结果/镜像 digest 待确认。
- PR / Commit：PR #131；merge commit `a0e878dbb43f796a8ca6aefa676b3d5be27dcc59`。
- 影响模块：Epusdt、GMPay Edge 支付状态查单与订单结算。
- 变更内容：
  - Epusdt 查询到已支付后，继续读取 checkout-counter-resp 快照并核对交易号、USDT、USD 和精确金额。
  - GMPay Edge 查单同样核验交易号、资产、法币币种和精确金额。
  - 资金字段缺失或不一致时记录异常、隔离支付意图并通知 Owner，不自动入账/退款。
- 数据库迁移：无。
- 配置/部署影响：无新增配置；无手工迁移。
- 兼容性与已知限制：真实商户环境的支付回调/查单验收仍需目标环境执行。
- 验证：
  - 自动化测试：PR head Go CI 通过，覆盖成功查单恢复及金额/资产错误与缺失场景。
  - 前端构建：Admin CI 通过。
  - 安全扫描：通过。
  - 生产支付端到端：未验收。
- 发布镜像：合并后 Docker Publish 已触发，digest 待 Actions 完成后补记。

## 2026-11-22 — PR #129 manual 商品买家自助购买已合并并发布（PROJECT_STATUS 2.2.82）

- PR #129 已 Squash Merge，merge commit `8a55d5959bb30bbec03006e0121298dc51c6294f`；实现 Telegram manual SKU 动态表单、私聊填写、20 分钟持久化会话、取消/重启恢复，以及 AES-GCM 加密订单表单快照。
- 付款后仅 manual 订单向 Dujiao-Next 转发 `manual_form_data`；auto 采购与本地商品购物车保持原样。当前只支持单件直购，manual 商品暂不支持购物车与优惠码。
- PR head Go/Admin/Security CI 全部成功；合并后 main Go/Admin/Security/Docker Publish 全部成功（runs：`44956658630`、`44956658628`、`44956658547`、`44956658620`）。
- GHCR 镜像标签：`latest` 与合并 commit SHA；OCI digest：`sha256:a1e26577afb22d7d285b4d69dfc7f61d235eb26c70a26122cee29dbaa4e47c02`。
- 生产部署、数据库迁移执行及真实 Telegram 买家购买验收仍待目标环境确认；CI 和镜像推送不等于生产验收。

## 2026-11-22 — main 状态校准（PR #125/#127 已合并；文档版本 2.2.80）

- PR #125 已合并，merge commit `830541e2c506875e0b92336dd06b92f31f04821e`；每日备份失败后按 30 分钟冷却重试，只有成功才记录当日完成。
- PR #127 已合并，merge commit `dfbf12ac744cd15f15b88ff08646d812a5163a44`；TRC20 过期订单人工补单必须填写核对依据，空备注在事务内拒绝且不改变交易/订单/库存。
- PR #127 合并后 main Go CI、Admin CI、Security Scan、Docker Publish 均成功（runs：`44954684292`、`44954684197`、`44954684146`、`44954684139`）。
- Docker Publish 已成功推送镜像；OCI digest 尚未独立核实，不记录未经核实的摘要。
- 生产恢复演练与真实支付环境验收仍单独跟踪；CI 通过不代表生产验收完成。

## 2026-11-22 — 全仓文档状态校准（PROJECT_STATUS 2.2.77）

- 本轮逐份复核仓库 Markdown 文档，统一支付、上游采购、售后验收和历史阶段状态；不修改业务代码。
- 当前支付状态：内置 TRC20 USDT、Epusdt、GMPay Edge 三个独立通道；PR #121/#122 已实现并发布下单、回调验签、查单、订单确认、幂等结算与交付交接。真实商户收款与回调可达性仍按目标部署环境单独验收。
- 当前上游状态：项目所有者确认 Dujiao-Next 正常对接完成，不再将 PR E 真实商户联调、额外扣款演练列为待办；异次元/萌次元不开发适配器。
- 当前售后状态：项目所有者于 2026-11-22 确认 Owner Bot C1–C4、媒体/附件转发、补发/退款分支及相关通知测试通过；旧待验收记录已改为历史阶段快照。
- 清理 PROJECT_STATUS、CHANGELOG、README、payment-delivery、support-tickets、upstream-integration、product-scope 中与当前事实冲突的描述；旧阶段记录保留并明确标注已被后续实现/验收覆盖。
- 文档版本递增至 2.2.77；本轮仅文档变更，独立 PR #123，不修改 main 业务代码。

<!-- 历史记录说明：本文件按时间倒序。下方旧条目中的“开发中/待验收/CI pending”等仅描述对应日期的阶段状态；若与顶部最新记录冲突，以顶部最新记录及 PROJECT_STATUS.md 当前状态为准。 -->

## 2026-11-22 — 后台菜单与三支付通道整合（历史阶段记录；PROJECT_STATUS 当时版本 2.2.76）

- 分支/PR：`feat/admin-menu-payment-provider-ux` / #122；已 Squash Merge，merge commit `0f916ffbbac1e95d38d6b96ecaf4d6afd9a0f5f8`。
- 后台侧边栏已按仪表盘、运营管理、商品管理、财务管理、系统设置分类；支付设置分别配置内置 TRC20 USDT、GMPay Edge、Epusdt，凭据相互独立。
- GMPay Edge 支持独立加密凭证、独立回调入口、签名查单和支付意图 provider 快照；新订单按启用通道轮询，支付意图 provider 固定，拒绝跨通道链上付款。
- Go CI run `44950987871`、Admin CI run `44950987936`、Security Scan run `44950987933`、Docker Publish run `44950987923` 均成功，均对应 main commit `0f916ffbbac1e95d38d6b96ecaf4d6afd9a0f5f8`。
- GHCR 标签：`ghcr.io/seanldnet/tg-card:latest` 和 commit SHA 标签；OCI digest：`sha256:89ba402c59ff45f4667ecd51d273507155fa814ece73e185e670065f0c03c551`。
- Go CI 首轮发现的测试夹具状态错位已修正：测试显式执行 `reserved → creating` 原子认领后再模拟网关响应；最新 PR head Go/Admin/Security 检查全部成功。
- 商城支付下单、回调验签、查单、订单确认、幂等结算及交付交接均已实现；真实商户三通道收款、回调可达性及端到端交付仍需按部署环境单独验收。
- 项目所有者于 2026-11-22 确认 Dujiao-Next 正常对接完成，不再将 PR E 真实商户联调列为待办；售后工单与 Owner Bot C1–C4、补发/退款及通知流程验收通过。

## 2026-11-22 — Epusdt GMPay 支付闭环实现（PR #121 已合并并发布；历史阶段记录）

- 分支：`feat/epusdt-gmpay-checkout-lifecycle-v2`；基线 main@`b96252cb7ea6fc0ee88badea7bb3dfaa029d7c16`。
- 实现商城商品订单与购物车统一订单 GMPay 收银台入口；后台配置 HTTPS 异步回调地址；订单金额以 USD 表示并指定 USDT/TRON 收款。
- 增加 Epusdt payment intent/event 持久化、GMPay HMAC-SHA256 回调验签、trade_id 状态查询、支付幂等与本地自动发货/上游采购交接。
- 异常处理：错误签名拒绝；金额或支付币种不符不确认；重复回调幂等；不同交易号的重复付款/意外交易转异常；订单取消后迟到付款不重开订单；创建超时进入 `create_uncertain`，禁止盲目重复创建。JSON 数字字段保留原始十进制文本，避免 float64 精度变化影响签名验算。
- 历史状态（PR #121 合并前）：当时 Go/Admin/Security CI 与 Docker Publish 尚未完成。后续 PR #121 已合并发布，CI 与镜像信息见 PROJECT_STATUS.md 顶部及本文件最新记录。

## 2026-11-22 — 文档基线彻底校准（PR #119，PROJECT_STATUS 2.2.71）

- PR / 分支：PR #119，`docs/calibrate-current-status-2.2.71`；基线 main@`82c5c0b14e18e8b117fcb7dda48bd7b7ff631d4f`。
- 校正 PR #114：每日自动备份与 Telegram Owner 投递已合并并发布，删除合并前“等待 CI/代码审查”的当前状态描述。
- 校正 PR #116/#117：GMPay 客户端与 Epusdt 商户配置均已合并；当时商城支付闭环仍待实现，后续已由 PR #121/#122 完成并发布。
- 统一 Dujiao-Next 状态：项目所有者已确认正常对接完成；不再将 PR E 真实商户验收作为待办或阻塞项。历史操作手册保留为参考。
- `PROJECT_STATUS.md` 文档版本递增至 2.2.71；本次仅更新文档，不修改业务代码。

## 2026-11-22 — Epusdt GMPay 文档状态校准

- 历史阶段状态：当时 PR #116/#117 已合并，但商城支付闭环尚待实现；该事项后续已由 PR #121/#122 完成并发布。
- `PROJECT_STATUS.md` 文档版本更新至 2.2.70。

## 2026-11-22 — 每日自动备份与 Telegram Owner 投递（PR #114 已合并并发布）

- 每日调度器默认 03:00（服务器本地时区），后台可启用/停用和调整时间。
- PostgreSQL `pg_dump` 与 `/app/data` 打包为 `.tar.gz`，通过 Telegram Bot `sendDocument` 投递给站点 Owner，并附 SHA-256。
- Docker runtime 包含 `postgresql-client`；保留手动备份/恢复脚本；无数据库迁移、无新增环境变量。
- PR #114 已合并并发布。此前“CI 与生产验收待完成”属于旧状态记录，现已更正；生产恢复演练单独跟踪。

## 2026-11-22 — 项目所有者确认 Telegram、售后、邮件与移动端测试通过

- 状态：项目所有者已确认本轮测试均无问题。
- 验收范围：Telegram 菜单/按钮、售后补发与退款状态分支、Owner Bot C1–C4 和失败场景、移动端真实设备回归、SMTP 实际邮件投递。
- 文档：同步将 PROJECT_STATUS.md 第 11 节对应项目更新为已验收，文档版本升至 2.2.67。
- 边界：Dujiao-Next 正常对接已由项目所有者另行确认完成；生产 PostgreSQL PITR/恢复演练仍是独立运维事项。
- 代码/数据库/配置：无业务代码变更、无数据库迁移、无新增配置。

## 2026-11-21 — SMTP STARTTLS 成功投递集成测试（PR #111 已合并并发布）

- 补充本地 SMTP 协议集成测试，覆盖 EHLO/STARTTLS 协商、TLS 握手、加密后 AUTH、MAIL、RCPT、DATA 与服务器接受邮件正文。
- 新增内部 TLS 配置注入 seam，仅用于信任测试用临时证书；生产仍使用系统根证书与 TLS 1.2 最低版本。
- 保留明文拒绝与 465 implicit TLS / 587 STARTTLS 模式选择测试。
- 无数据库迁移、无新增环境变量或后台配置；真实网易/QQ 邮箱投递仍需部署后实发验收。
- PR #111 head Go/Admin/Security CI 全部成功；合并后 main Go/Admin/Security/Docker Publish 全部成功。GHCR OCI manifest digest：`sha256:0ce229136794ce1ba60c981f07f635fd700c3a39c77b6784e3fe5e99de13ad26`。
- 无数据库迁移、无新增环境变量或后台配置；真实网易/QQ 邮箱投递仍需部署后实发验收。PROJECT_STATUS 文档版本更新至 2.2.66。

## 2026-11-21 — P0-3/P0-4 分步骤交付与资金保护状态校准（PR #84–#89 已合并）

- PR #84：分步骤 file_card 交付状态机已合并，merge commit `c56d6322a6a16bd5ce71977e52152a82eeb9d1c1`。
- PR #85：已付款上游订单缺失采购队列恢复已合并，merge commit `1a6f05cde287ecfd5ac52a33e02698d70d75bc63`。
- PR #86：交付结果不确定时按订单幂等通知 Owner 已合并，merge commit `3e2be62cef60553e92a92aea082d2470a51bf3e9`。
- PR #87：退款状态双向一致性只读核对已合并，merge commit `ccb1803a6418a930c1cdf091607c41a31e508b07`。
- PR #88：已退款上游订单缺少采购生命周期记录检测已合并，merge commit `4ee33774a6bc136f3360728a91a58d5dfd43f4eb`。
- PR #89：活动首次交付期间禁止退款已合并，merge commit `f3e09ba7637f683494a8675b3f7e928df59059b9`；Docker Publish digest `sha256:ecbcb456eb6e2eb19057d3338b2705ac09e7d0e6a563a338096340eff6d71eb9`。
- 安全边界：不确定交付/采购不盲目重试，不自动退款或调整钱包；财务一致性巡检只登记异常，不自动修复。
- 生产 PostgreSQL 历史数据检查、真实 Telegram 不确定发送/通知与交付-退款并发验收仍待目标环境执行。

## 2026-11-21 — SMTP TLS 发送修复（PR #107 已合并并发布）

- 根因：站点配置中的 `SMTPUseTLS` 未应用到测试邮件、管理员邮箱验证码和业务通知的 SMTP 发送路径。
- 修复：统一 SMTP 客户端；465 隐式 TLS、支持 STARTTLS 的端口先协商加密；启用 TLS 但服务器不支持加密时 fail closed；禁止 SMTP 凭据通过明文连接认证。
- 后台补充网易 yeah.net/163 与 QQ SMTP 地址、完整邮箱账号及授权码说明。
- PR head Go/Admin/Security CI 全部成功；合并后 main Go/Admin/Security/Docker Publish 全部成功，GHCR manifest digest：`sha256:b8c84ea9241472750b6301e8f66cf3ead4aeb59862bc208bd620dad0173f9c1f`。
- 无数据库迁移、无新增环境变量；真实邮箱投递验收待部署后执行。PROJECT_STATUS 文档版本更新至 2.2.62。

## 2026-11-21 — 登录双重验证按账户状态显示与 Cap 验证组件紧凑化（PR #105 已合并并发布）

- PR #105 已 Squash Merge，merge commit `84ead9a94505c1c0b9a9240b0748d9472dfdf048`。
- 登录按账户 TOTP 状态条件显示二次验证字段；恢复码复用同一输入框；Cap widget 高度缩至 30px 并保持全宽对齐。
- PR head Go/Admin/Security CI 与合并后 main Go/Admin/Security/Docker Publish 全部成功；GHCR manifest digest：`sha256:bf5a1ba71e51cfd9318e3feada105c3f741d90bb4d6bb52bd854d7f5968e71a3`。
- 真实手机浏览器视觉验收仍待执行。

## 2026-11-21 — Owner Bot 媒体回复限制为单次 Telegram 副作用（PR #104 已合并；原 PR #77 已关闭并由 PR #104 替代）

- 状态：基于 PR #103 合并后的 main 新建独立分支 `fix/owner-media-reply-single-side-effect-rebased`，不沿用落后主线的旧 PR 分支。
- 影响模块：Owner Bot 售后媒体回复、客服工单时间线。
- 变更内容：caption 超过 900 个 Unicode 字符时，在任何客户侧媒体发送前拒绝；移除媒体发送后再补发剩余文本的第二次 Telegram API 调用。
- 数据库迁移：无；配置/部署影响：无新增环境变量。
- 验证：PR head Go/Admin/Security CI 已通过；项目所有者已确认 Telegram、售后与 Owner Bot C1–C4 测试通过（2026-11-22）。
- 兼容性与验收边界：普通文本回复和用户提交媒体不变；项目所有者已确认 Telegram、售后与 Owner Bot C1–C4 测试通过（2026-11-22）。

## 2026-11-21 — 备份恢复集成测试补充模拟 RPO/RTO 计时（PR #102 已合并并发布）

- 状态：PR #102 已 Squash Merge 至 main，merge commit `a3e26c5ea4402b6f79876c7aa5ac4f43fed6e337`。
- 影响模块：隔离 Docker Compose 备份恢复集成测试、运维验收指标。
- 变更内容：测试记录备份完成时刻至模拟数据丢失时刻的模拟 RPO，以及恢复脚本启动至隔离 app 容器 running 的容器启动 RTO。
- 数据库迁移：无；生产备份/恢复脚本无改动。
- 验证：PR head Go Backend CI、Admin Frontend CI、Security Scan 全部成功；合并后 main Go run `44889690716`、Admin run `44889690674`、Security run `44889690676`、Docker Publish run `44889690723` 全部成功。
- 发布镜像：GHCR `latest` 与 commit tag `a3e26c5ea4402b6f79876c7aa5ac4f43fed6e337` manifest digest：`sha256:aaba213e22f07fd64827ffd2046da4c69f14c1340df41616428c3cc7fbde4396`。
- 边界：模拟 RPO 不代表生产备份周期的真实 RPO；容器 running 不等于应用健康就绪，生产恢复演练、PostgreSQL PITR 与业务 smoke test 仍待执行。PROJECT_STATUS 文档版本更新至 2.2.58。


## 2026-11-21 — P1 管理员敏感操作二次确认（PR #100 已合并）

- 状态：PR #100 已 Squash Merge，merge commit `50ecad1ca9fd72ec319e46b1d26fd659a3ad8f34`；合并后 Go/Admin/Security CI 全部成功，Docker Publish run `44885024397` 已成功；GHCR `latest` 与 commit tag `50ecad1ca9fd72ec319e46b1d26fd659a3ad8f34` manifest digest：`sha256:1ef5ec7d1ef7cc75b9b764bcbea972b83687798468f705e0a4da3664704493a2`。
- 影响模块：管理员安全认证、余额/采购/退款/支付/订单敏感 API、Admin UI。
- 变更内容：二次确认需当前管理员密码；启用 TOTP 时还需动态验证码或一次性恢复码。2 分钟一次性凭证仅存 SHA-256 哈希，绑定管理员、action key、method、path 与规范化请求体；后端原子消费防重放；前端处理 428 challenge 并重放原请求。
- 数据库迁移：新增 `0057_admin_action_confirmations.sql`；启动时自动迁移，无手工 SQL。
- 配置/部署影响：无新增环境变量；需部署新镜像后才启用新接口与后台交互。
- 兼容性与已知限制：真实浏览器密码/TOTP/恢复码确认流程仍待部署验收；CI 通过不代表生产验收。
- 验证：PR head Go Backend CI、Admin Frontend CI、Security Scan 全部通过；合并后 main Go Backend CI、Admin Frontend CI、Security Scan 全部通过；Docker Publish（run `44885024397`）成功，GHCR manifest digest：`sha256:1ef5ec7d1ef7cc75b9b764bcbea972b83687798468f705e0a4da3664704493a2`。真实浏览器 step-up 流程仍待部署验收。

## 2026-11-21 — 数据库备份/恢复隔离演练自动化（PR #101 已合并并发布）

- 状态：PR #101 已 Squash Merge 至 main，merge commit `bd1087fc92ea5dd0263ebaa62a6ea94cb8f12f6d`；基于 PR #100 合并后的 main。
- 影响模块：Compose 备份脚本、恢复脚本、PostgreSQL 数据、应用持久化密钥与上传文件、Go CI。
- 变更内容：新增隔离 Docker Compose 集成测试，调用正式 `scripts/backup.sh` 与 `scripts/restore.sh`；创建合成分类、商品、库存卡密、订单及站点设置记录，以及 JWT/加密密钥文件和上传文件；备份后模拟订单状态污染、库存行丢失、设置变更、密钥损坏和上传文件丢失，再恢复并断言所有状态回到备份值。
- 数据库迁移：无生产 schema 迁移；测试使用临时 PostgreSQL 16 容器和独立命名卷。
- 配置/部署影响：无生产环境变量；CI runner 需要 Docker Compose。
- 兼容性与已知限制：本测试 RTO 只衡量隔离测试容器恢复到 running 的耗时，不代表生产应用就绪 RTO；生产 RPO/RTO 需按实际备份频率和业务验收单独测量。
- 验证：PR head Go/Admin/Security CI 全部通过；合并后 main Go/Admin/Security CI 与 Docker Publish run `44886025299` 全部成功；GHCR `latest` 与 commit tag `bd1087fc92ea5dd0263ebaa62a6ea94cb8f12f6d` manifest digest：`sha256:ee2c1611a8d7635fe9ae5c1ea5d504d33168e9c5a34ff819a71ed853fb5020b6`。隔离测试通过；真实生产备份/恢复演练未执行。PROJECT_STATUS 文档版本更新至 2.2.56。


## 2026-11-21 — 管理员登出即时撤销 Access Session（PR #99 已合并并发布）

- 修复 Vben Admin 默认登出请求未携带 Authorization Bearer 的问题；现在同时发送 Bearer 与 HttpOnly Refresh Cookie。
- 登出后服务端撤销当前 access session 和整个 refresh token family；新增 HTTP 回归测试验证旧 access token 立即失效。
- 无数据库迁移、无新增环境变量；该阶段 CI 待验证属于当时状态，后续验证结果以对应 PR 与 main Actions 记录为准。

## 2026-11-21 — P1 管理员短效 Access Token 与轮换 Refresh Token（PR #97 已合并并发布）

- Access Token 有效期缩短至 15 分钟；7 天 Refresh Token 使用 HttpOnly、SameSite=Lax Cookie。
- 新增 refresh endpoint，兼容 Vben Admin 内置刷新客户端；每次刷新轮换令牌，数据库仅存哈希。
- 检测 refresh token 重放后撤销整个会话族；登出撤销 refresh family，密码/TOTP 状态变更通过 token_version 使旧 refresh token 失效。
- 新增迁移 `0056_admin_refresh_sessions.sql`，无新增环境变量。Go/Admin/Security CI 与浏览器验收待完成；PROJECT_STATUS 文档版本更新至 2.2.52。

## 2026-11-21 — P1 TOTP 一次性恢复码（PR #96 已合并）

- 为管理员 TOTP 绑定增加 10 个一次性恢复码；仅在绑定确认时显示一次，数据库只存 SHA-256 哈希。
- 登录可使用动态验证码或未使用的恢复码；恢复码采用并发安全的一次性消费，重复使用会拒绝并记录审计。
- 关闭 TOTP 清空恢复码；安全设置页面显示一次性恢复码并提示安全保存。
- 新增迁移 `0055_admin_totp_recovery_codes.sql`；PR #96 Go/Admin/Security CI、SQLite/PostgreSQL 测试及 Docker Publish 全部通过并已合并；GHCR manifest digest：`sha256:41c3c879a278a7aa442f45abd838abfb6fa82df718515a4ed2811d30d97a8758`。PROJECT_STATUS 文档版本更新至 2.2.51。

## 2026-11-21 — P1 管理员 TOTP 双重验证（PR #95 已合并）

- 新增管理员 TOTP 绑定、动态码确认、关闭与登录校验；密钥 AES-GCM 加密存储，绑定前需当前密码。
- 新增登录动态码字段、安全设置页面、接口限流与审计。
- 迁移 0054_admin_totp.sql；无新增环境变量。PR #95 Go/Admin/Security/Docker Publish 全部通过并已合并；GHCR manifest digest：`sha256:b51ec8ed881cf73d52e2038e6f68a3064b3c818b0e59132970df2339b77fa17a`。真实管理员绑定验收待完成。PROJECT_STATUS 文档版本更新至 2.2.50。

## 2026-11-21 — P1 业务风险 Owner 主动告警（PR #94 已合并）

- 新增每分钟风险巡检：连续 3 笔交付失败、退款账本冲突达到 3 笔、交付/采购 uncertain 等待达到 15 分钟、Cap 连续失败 3 次、API P95 高于 1500 ms、DB 连接池耗尽。
- 告警复用 `risk_alert`，按告警类型每小时幂等去重；不触发自动重试、退款或余额修正。
- 无数据库迁移和新增配置；PR #94 Go/Admin/Security CI 全部通过并已合并；合并后 main 发布工作流待最终确认。PROJECT_STATUS 文档版本更新至 2.2.49。

## 2026-11-21 — P1 业务数据大盘与运行指标（PR #93 已合并）

- 新增管理员鉴权的经营指标 API：交付成功率/不确定订单 ageing、退款账本冲突、上游同步进度、Cap 成功失败计数、API P95、数据库连接池。
- 经营概览新增对应指标卡片，基础业务概览保留。
- Cap 与 API 延迟使用进程内计数/采样；进程重启后重置。长期趋势与 Owner 告警阈值后续独立实现。
- 新增聚合查询回归测试；无数据库迁移、无新增配置。PR #93 Go/Admin/Security CI 与合并后 Docker Publish 均成功；GHCR manifest digest：`sha256:a4faef34507ff02cb73bf058f5261846d27714e9fa96456905413f6d5881da9c`。PROJECT_STATUS 文档版本更新至 2.2.48。

## 2026-11-21 — P0-4 上游采购 uncertain 人工核实入口（PR #92 已合并）

- 上游设置页对 uncertain 采购订单改为仅显示“核实受理状态”，不再提供直接重试/退款按钮。
- 复用既有人工核实 API：供应商确认已受理时登记上游订单 ID 并恢复查单；确认未受理/明确失败时记录核实依据，收敛为 failed 后再允许重试/退款。
- 新增回归测试覆盖 not_accepted 核实后沿用原本地订单幂等键安全重试，且不创建退款流水。
- 无新增 API、数据库迁移或环境变量；PR #92 Go/Admin/Security CI 与合并后 Docker Publish 均成功；GHCR manifest digest：`sha256:52fc62675515d150e3fdd1f12d72428099e0c60b36b3e3e4133676fa1551cf4c`。PROJECT_STATUS 文档版本更新至 2.2.47。

## 2026-11-21 — P0-4 不确定交付禁止盲目手动重试（PR #91 已合并）

- 普通卡密、固定文本、上游内容等单步骤交付处于 `delivery_uncertain` 时，管理员重试入口拒绝再次发送。
- 数据库 claim 层增加保护：只有所有步骤均已明确核实的 file_card 订单可恢复；缺少步骤记录或仍有 uncertain/sending 步骤时拒绝。
- 新增 SQLite/PostgreSQL 回归测试覆盖普通单步骤不确定订单拒绝手动 claim，以及 file_card 步骤核实前后恢复边界。
- 无数据库迁移、无新增配置；PR #91 Go/Admin/Security CI 与合并后 Docker Publish 均成功；GHCR manifest digest：`sha256:9034d2f3ecbad205291345e84e34f6c9c9e499c3022f68d615b3b1a705bccadf`。PROJECT_STATUS 文档版本更新至 2.2.46。

## 2026-11-21 — P0-4 上游采购不确定状态资金保护（PR #90 已合并并发布）

- 禁止对供应商受理状态不确定（`uncertain`）的订单直接手动重试，避免供应商已受理时重复采购。
- 禁止对 `uncertain` 订单直接执行钱包退款，避免供应商已履约与买家退款并存。
- 只有采购队列明确为 `failed` 的订单才能进入现有手动重试/退款流程；uncertain 必须先由 Owner/管理员在供应商侧核实。
- 新增回归测试验证重试、退款均拒绝且不触发第二次上游创建、不产生退款账本。
- 无数据库迁移、无新增配置；PR #90 Go/Admin/Security CI 及合并后 Docker Publish 均成功；GHCR manifest digest：`sha256:23a5aa26f511a5f07609a262d4c1e7a451e84c30a98e83041b61d66e26be109c`。PROJECT_STATUS 文档版本更新至 2.2.45。

## 2026-11-21 — P0-4 首次交付期间退款保护（PR #89 已合并并发布）

- 收紧 `RefundOrderToWallet` 状态门禁：`delivering` 不再允许退款，避免首次 Telegram 交付执行期间并发给买家余额入账。
- 退款仅允许在 `paid`、`delivery_failed`、`delivered` 等明确状态继续；`delivery_uncertain`、`delivering`、`refunded` 和其他非法状态拒绝。
- 新增 SQLite/PostgreSQL 共用回归测试，断言活动交付期间退款不改变订单状态、钱包余额或退款流水。
- 无数据库迁移、环境变量或 API 契约变化；PR #89 Go/Admin/Security CI 及合并后 Docker Publish 均成功；GHCR manifest digest：`sha256:ecbcb456eb6e2eb19057d3338b2705ac09e7d0e6a563a338096340eff6d71eb9`。PROJECT_STATUS 文档版本更新至 2.2.44。

## 2026-11-21 — P0-4 上游退款生命周期孤立订单检测（当日开发阶段快照；后续状态见后续合并记录）

- 新增只读核验：已退款且来源快照明确为 upstream 的订单，必须存在对应 `upstream_procurements` 生命周期记录。
- 若缺少采购生命周期记录，登记 critical 财务异常，提示无法核对供应商退款状态。
- 新增回归测试，确认检测器不补建采购记录、不修改订单状态。
- 历史状态：该阶段 PR CI 当时待验证；后续合并与发布结果见本文件更新记录及 PROJECT_STATUS.md 对应 PR 记录。

## 2026-11-21 — P0-4 退款状态一致性核对（当日开发阶段快照；后续状态见后续合并记录）

- 扩展只读退款完整性检查：检查本地订单与上游采购队列是否同时标记为 `refunded`。
- 检测订单已退款但采购队列未退款，以及采购队列已退款但订单未退款两类状态分裂，并写入财务异常中心。
- 新增回归测试覆盖双向状态不一致，并验证检查器不会修改订单或采购状态。
- 历史状态：该阶段 PR CI 当时待验证；后续合并与发布结果见本文件更新记录及 PROJECT_STATUS.md 对应 PR 记录。

## 2026-11-21 — P0-4 交付结果不确定 Owner 告警（当日开发阶段快照；后续状态见后续合并记录）

- Telegram 交付请求已发出但网络/响应不确定时，继续将订单保持在 `delivery_uncertain` 隔离状态，并更新异常原因。
- 新增按订单 ID 唯一认领的一次性 Owner 风险告警；复用现有 `risk_alert` 通知配置，不在租约恢复或重复扫描时重复发送。
- 告警包含本地订单号、结果不确定原因和人工核实提示；不包含卡密、支付凭据或 Telegram Bot Token。
- 新增测试验证同一订单重复触发只通知一次。真实 Telegram 网络响应丢失场景待部署验收。

## 2026-11-21 — 订单与资金补偿机制 P0-4（当日开发阶段快照；后续状态见后续合并记录）

- 新增 `ReconcileMissingUpstreamProcurements`：只恢复已付款、状态仍为 `procurement_pending`、来源快照为 upstream 且缺少采购队列行的订单。
- 恢复队列沿用本地订单号/订单 ID 幂等键，并以原始 `paid_at` 加采购超时配置计算 deadline；不会因恢复操作延长采购期限。
- 调度器每轮领取采购任务前先执行缺失队列扫描；仅补建队列，不修改订单支付信息、钱包余额或资金账本。
- 新增回归测试覆盖恢复、重复扫描幂等、支付信息不变和钱包账本不变。PR head CI 结果待验证。

## 2026-11-21 — 分步骤交付状态机 P0-3（当日开发阶段快照；后续状态见后续合并记录）

- 状态：独立分支 `feat/stepwise-delivery-state-machine-p0-3`，基于 main@0a8895d54bbac02a78743b8692192c8ed66ea072。
- 背景：PR #71 已阻止不确定 file_card 整单重试，防止重复发送已成功文件，但也导致前一步成功、后一步明确失败时只能整单人工介入。
- 变更：新增 `order_delivery_steps` 与迁移 0053；分别持久化文件发送和卡密消息发送状态、尝试次数、Telegram message_id、错误和时间。
- 恢复：仅对明确失败步骤重试，已成功步骤跳过；传输/响应不确定或崩溃遗留 sending 的步骤必须先人工核实并写入审计；只有所有步骤成功才允许订单完成。
- 并发保护：发送期间保留订单租约，阻止并发 claim/人工核实；租约过期后的遗留 sending 状态先转 uncertain，再人工核实。
- 后台：订单详情显示每个步骤的状态、attempts、message_id 和错误；支持对不确定步骤进行有审计的人工成功/失败判定。
- 安全边界：不改变已交付 file_card 再次获取限制；不声称 Telegram exactly-once。
- 验证：Go/Admin/Security CI 待本 PR 检查；真实 Telegram 多步骤恢复待真机验收。

## 2026-11-21 — 全局异常中心 P0-2（PR #83 已合并并发布）

- 状态：PR #83 已 Squash Merge 至 main，merge commit `0a8895d54bbac02a78743b8692192c8ed66ea072`；原冲突 PR #80 已关闭，由 PR #83 基于最新 main 重整。
- 统一 `system_exceptions` 表及迁移 0052；迁移历史 `finance_exceptions` 并保留旧表；统一采购、供应商资金差异、退款账本、交付不确定、退款冲突、上游同步失败和 Cap 异常登记。
- 新增后台全局异常中心、按状态/类型/级别/时间筛选、处理中/解决/忽略操作；状态更新与 `system.exception.update` 审计在同一事务提交。
- PR head Go CI #44836448199、Admin CI #44836448185、安全扫描 #44836448125 全部成功；合并后 Go CI #44836648477、Admin CI #44836648431、安全扫描 #44836648441、Docker Publish #44836648433 全部成功。
- GHCR `latest` 与提交标签 `0a8895d54bbac02a78743b8692192c8ed66ea072` 已发布，manifest digest：`sha256:9792e121f9447fc450921910d3bafab545119ea8d67de1702ee8cc49c3d1a06d`。
- 安全边界：异常详情不记录 Cap Secret Key 或一次性 Token；标记解决/忽略只更新异常登记状态和审计，不自动修改订单、钱包、库存或供应商余额。真实生产 PostgreSQL 历史迁移和异常处理流程仍待部署验收。

## 2026-11-21 — Cap 登录验证框布局与中文文案修复（PR #82 已合并）

- 状态：PR #82 已 Squash Merge 至 main，merge commit `d083aafe2cf63017f5583f3c702defc1c3774032`。
- 影响模块：管理员登录页、网站设置 Cap 配置页。
- 变更内容：Cap Widget 放在密码字段下方并占满表单宽度；初始提示“点击进行安全验证”，验证中/成功/失败提示中文化；删除 Cap 设置页两条冗余提示框。
- 安全边界：保留后端一次性 Token 校验、Secret Key 加密存储和 fail-closed 行为。
- 验证：合并后 Admin Frontend CI run `44835908943` 已成功；Docker Build and Publish run `44835909014` 正在构建推送。真实 Cap 浏览器 E2E 仍待项目所有者验收。
- 发布镜像：等待 Docker Publish 完成后确认。

## 2026-11-21 — Cap Widget 新版 CDN 加载兼容修复（PR #81 已合并）

- 状态：PR #81 已 Squash Merge 至 main，merge commit `791f3d43a623fdb3e59ba378626890083bf5aebd`。
- 影响模块：管理员登录页、网站设置 Cap 配置说明、Cap 专项运维文档。
- 变更内容：登录页默认加载固定版本 jsDelivr `cap-widget@0.1.63`；不再依赖 Cap 服务的 `/assets/widget.js` 或「Widget files」开关；保留服务端 `/{site-key}/siteverify` 校验与 fail-closed 行为。
- 验证：合并后 Admin Frontend CI run `44833799359` 与 Docker Build and Publish run `44833799370` 均成功；Docker Build and push 步骤成功。真实 Cap E2E 尚未验收。
- 发布镜像：workflow 已成功推送；manifest digest 尚未在本次记录中核实，不作猜测。


## 2026-11-21 — Cap 自托管人机验证接入（PR #78 已合并）

- 状态：已 Squash Merge 至 main，merge commit `8981fb887584979450f19b30648225ced113989d`。
- 影响模块：管理员登录、网站设置、服务端密钥管理、数据库迁移、Docker Compose 运维配置。
- 变更内容：
  - 新增 Cap 设置：服务域名、Site Key、Secret Key；Secret Key 使用 AES-256-GCM 加密，后台只返回 configured 状态。
  - 登录页动态加载 Cap 自托管 `/assets/widget.js` widget，获取一次性 token 后随登录请求提交。
  - 后端 POST `/{site-key}/siteverify`，发送 `secret` 与 `response`，只有 `success=true` 放行；验证服务异常时 fail-closed。
  - 新增数据库 `settings` 键值表，支持指定 SQL 紧急关闭；Compose 增加 `DISABLE_CAP` 环境变量覆盖开关。
- 数据库迁移：新增 `0051_system_settings.sql`，兼容 PostgreSQL/SQLite；Cap Secret 密文依赖持久化 `DATA_DIR/bot-token.key`。
- 配置/部署影响：Cap 服务需启用静态资源并提供 `/assets/widget.js`；浏览器与 app 容器均需访问 Cap HTTPS 域名。紧急 SQL 命令和环境变量重建方式见 `docs/cap-human-verification.md`。
- 兼容性与已知限制：未配置时默认关闭，不影响现有登录；当前保护管理员登录，其他敏感表单后续复用验证器。拒绝 HTTP、localhost 和私有 IP Cap 目标。
- 验证：PR head Go/Admin/Security 全部成功；合并后 main Go Backend CI #44828737475、Admin Frontend CI #44828737466、Security Scan #44828737403、Docker Publish #44828737396 全部成功。GHCR 多架构 `latest` 与提交标签 `8981fb887584979450f19b30648225ced113989d` 已发布，manifest digest：`sha256:e062f1dd8f77c4e3b515ded826668ad5fc89e344da429d59e102d7803a3587dc`。
- 真实 Cap 服务联调尚未执行；CI 通过不代表真实域名上的浏览器挑战与登录验收完成。
- 文档：PROJECT_STATUS 文档版本更新至 2.2.35；新增 `docs/cap-human-verification.md`。

## 2026-11-21 — 再次获取期间退款冲突审计与 HTTP 状态（PR #75 已合并并发布）

- 状态：已 Squash Merge 至 main，merge commit `35e20d5810ce140e57c2dde96f9b0b4af4f10fc9`；开发基线 main@9abf51599e1c93d28c8cac30546e8ef5976bd653。
- 影响模块：售后退款 API、管理员审计日志、订单再次获取租约冲突。
- 变更内容：对活动再次获取租约导致的退款拒绝返回 HTTP 409，并写入 `support.refund.blocked` 审计事件；其他退款失败写入 `support.refund.failed`。使用数据库 sentinel error 做精确分类。
- 数据库迁移：无；配置/部署影响：无新增环境变量。
- 兼容性与已知限制：活动租约结束后管理员可重新发起退款；不改变钱包退款业务逻辑。
- 验证：PR head Go/Admin/Security 均成功；合并后 main Go #44814346690、Admin #44814346681、Security #44814346722、Docker Publish #44814346693 全部成功。项目所有者已确认售后测试通过（2026-11-22）。
- 发布镜像：GHCR 多架构标签 `latest` 与提交标签 `35e20d5810ce140e57c2dde96f9b0b4af4f10fc9` 已发布；manifest digest：`sha256:902cfecbc14e8d2d6eaed5c05ff1b01019343b4d456e1b24e17224e06b5a4dd8`。

## 2026-11-21 — 退款与订单再次获取租约互斥（PR #74 已合并并发布）

- 状态：已合并至 main；merge commit `9abf51599e1c93d28c8cac30546e8ef5976bd653`。
- 影响模块：钱包退款、已交付订单再次获取、数据库订单租约。
- 变更内容：修复退款与再次获取并发窗口。再次获取领取租约前锁定订单行；退款在同一订单行锁下检查活动再次获取租约，活动期间拒绝退款；租约过期后退款可继续，退款完成后再次获取拒绝。
- 数据库迁移：无，复用现有 `order_redelivery_leases` 表。
- 配置/部署影响：无新增环境变量；无需手工 SQL。
- 兼容性与已知限制：活动再次获取租约期间管理员退款会返回错误，需要等待租约结束后重新操作；不改变钱包退款金额和账本事务。
- 验证：PR head Go #44812800729、Admin #44812800826、Security #44812800803 全部成功；合并后 main Go #44813061227、Admin #44813061231、Security #44813061385、Docker Publish #44813061197 全部成功。生产部署与真实退款/再次获取验收仍需项目所有者执行。
- 发布镜像：GHCR 多架构标签 `latest` 与提交标签 `9abf51599e1c93d28c8cac30546e8ef5976bd653` 已发布；manifest digest：`sha256:3392d6b4d4c2150bd95d06093082802865d02e86e017a060faa97bea46c1d0c0`。

## 2026-11-21 — PostgreSQL 再次获取租约并发回归（PR #73 已合并并发布）

- 状态：已合并至 main；merge commit `21cfd3ffc88606bc31ceb8c3a0bc3a77002cdbca`。
- 影响模块：订单再次获取数据库租约、PostgreSQL 并发集成测试。
- 变更内容：为 `ClaimOrderRedelivery` 增加 PostgreSQL 16 并发回归测试，验证 8 个 worker 争抢同一已交付订单时只有一个获胜；活动租约拒绝再次领取，过期租约允许恢复。
- 数据库迁移：无；仅测试与文档变更。
- 配置/部署影响：无生产配置变化；复用 Go CI 的临时 PostgreSQL 16 服务。
- 兼容性与已知限制：不改变再次获取租约算法或业务行为。
- 验证：PR head Go [#44811797126](https://github.com/seanldnet/TG-Card/actions/runs/44811797126)、Admin [#44811797029](https://github.com/seanldnet/TG-Card/actions/runs/44811797029)、Security [#44811796942](https://github.com/seanldnet/TG-Card/actions/runs/44811796942) 全部成功；合并后 main Go [#44812154123](https://github.com/seanldnet/TG-Card/actions/runs/44812154123)、Admin [#44812154882](https://github.com/seanldnet/TG-Card/actions/runs/44812154882)、Security [#44812154255](https://github.com/seanldnet/TG-Card/actions/runs/44812154255)、Docker Publish [#44812154449](https://github.com/seanldnet/TG-Card/actions/runs/44812154449) 全部成功。
- 发布镜像：测试/文档 PR，不改变运行时镜像内容。

## 2026-11-21 — 已交付 file_card 再次获取防重复发送（PR #72 已合并并发布）

- 状态：已合并至 main；merge commit `5d56777a3d24d5ff3e4afb0cfa746c3ac9ffea3b`。
- 影响模块：Telegram 用户订单再次获取、Telegram Manager、管理员订单重发 API。
- 变更内容：阻止已交付 `file_card` 订单自动再次获取，用户订单列表/详情引导联系客服核实；管理员 API 返回 HTTP 409 并写入审计；单步骤交付类型保持既有行为。
- 数据库迁移：无；配置/部署影响：无新增生产环境变量。
- 验证：PR head Go [#44810969393](https://github.com/seanldnet/TG-Card/actions/runs/44810969393)、Admin [#44810969458](https://github.com/seanldnet/TG-Card/actions/runs/44810969458)、Security [#44810969354](https://github.com/seanldnet/TG-Card/actions/runs/44810969354) 全部成功；合并后 main Go [#44811290066](https://github.com/seanldnet/TG-Card/actions/runs/44811290066)、Admin [#44811290206](https://github.com/seanldnet/TG-Card/actions/runs/44811290206)、Security [#44811290095](https://github.com/seanldnet/TG-Card/actions/runs/44811290095)、Docker Publish [#44811290150](https://github.com/seanldnet/TG-Card/actions/runs/44811290150) 全部成功。
- 发布镜像：GHCR 多架构标签 `latest` 与提交标签 `5d56777a3d24d5ff3e4afb0cfa746c3ac9ffea3b` 已发布；manifest digest：`sha256:e4bcea3b8d21090ce5d976368bd2996129ad36c50d02edcfc0771d78f09038ba`。生产部署和 Telegram 真机验收未确认。

## 2026-11-21 — 多步骤交付不确定结果禁止整单重试（PR #71 已合并并发布）

- 状态：已合并至 main；merge commit `cdb30cef99f27e3021de35fe1717937becc116c5`。
- 影响模块：Telegram 订单交付、管理员订单重试、订单列表/移动端卡片。
- 变更内容：阻止 `delivery_uncertain` 的 `file_card` 整单重试，保留一次性 `invite_link` 防护；增加 Manager 与管理员 API 回归覆盖。
- 数据库迁移：无；配置/部署影响：无新增生产环境变量。
- 验证：PR head Go [#44809724292](https://github.com/seanldnet/TG-Card/actions/runs/44809724292)、Admin [#44809724257](https://github.com/seanldnet/TG-Card/actions/runs/44809724257)、Security [#44809724481](https://github.com/seanldnet/TG-Card/actions/runs/44809724481) 成功；合并后 main Go [#44810140754](https://github.com/seanldnet/TG-Card/actions/runs/44810140754)、Admin [#44810140828](https://github.com/seanldnet/TG-Card/actions/runs/44810140828)、Security [#44810140787](https://github.com/seanldnet/TG-Card/actions/runs/44810140787)、Docker Publish [#44810140725](https://github.com/seanldnet/TG-Card/actions/runs/44810140725) 全部成功。
- 发布镜像：GHCR 多架构标签 `latest` 与提交标签 `cdb30cef99f27e3021de35fe1717937becc116c5` 已发布；manifest digest：`sha256:926ee2f545d6c26580a0d453eefdb0df3ba97a4271b09c5505eb078603f0b63d`。生产部署和 Telegram 真机验收未确认。

## 2026-11-21 — 一次性邀请链接不确定结果禁止自动重试（PR #70 已合并并发布）

- PR #70 已 Squash Merge 至 main，merge commit `d7cf00e783c3e97b2ecee77d329488e3091a220c`。
- Telegram `createChatInviteLink` 可能已创建有效链接但响应丢失或本地持久化失败；API 与 Manager 双重阻止 `delivery_uncertain` 的 `invite_link` 重试，管理员列表隐藏重试并显示“需人工核实”，API 拒绝操作写审计。
- 合并后 main Go CI #44804736581、Admin CI #44804736573、Security #44804736340、Docker Publish #44804736390 全部成功。
- GHCR 多架构镜像标签 `latest` 与提交标签 `d7cf00e783c3e97b2ecee77d329488e3091a220c` 已发布；manifest digest：`sha256:1c2a7df3ace81cd4af6162ff2501892922508df611ad9e7699f2acf9318b059a`。
- 生产部署与 Telegram 真机验收仍需项目所有者确认。

## 2026-11-21 — Telegram 订单交付不确定结果隔离（PR #69 已合并并发布）

- PR #69 已 Squash Merge，merge commit 1325e52443c0f06d52d21205c2e29af5fca39b70。
- 首次 Telegram 外部交付副作用前持久化 delivery_uncertain，自动 worker 排除该状态；发送成功后可收敛为 delivered。管理员核实后可手动处理，普通本地错误保留指数退避。
- PR head Go/Admin/Security 通过；合并后 main Go/Admin/Security/Docker Publish 全部成功。
- GHCR manifest digest：sha256:48e900b0a06a4e346eb36a5fc9cd51834041df0a8abb4442908d962e1a947a6f。
- 生产部署与 Telegram 真机验收仍待项目所有者确认。
## 2026-11-21 — 订单交付失败指数退避（PR #67 已合并并发布）

- PR #67 已 Squash Merge 至 main，merge commit `c460706a1cc7675cfbd9f274a9311f3f2654f468`。
- 后台自动交付 worker 对 `delivery_failed` 订单增加指数退避：15、30、60、120、240、480 秒，之后封顶 900 秒；首次付款交付与过期租约恢复不受影响，管理员手动补发仍立即执行。
- SQLite 与 PostgreSQL 16 集成测试覆盖退避时序和手动补发旁路；PR head Go/Admin/Security 全部成功。
- main Go CI #44784753138、Admin CI #44784752882、Security #44784753131、Docker Publish #44784753246 全部成功。
- GHCR 多架构 `latest` 与提交标签 `c460706a1cc7675cfbd9f274a9311f3f2654f468` 已推送；manifest digest：`sha256:7c5710db0edf473ca828022030f9b3904ad871e32316cb728ae15222648443eb`。
- 无数据库迁移或新增生产环境变量；生产部署状态需项目所有者确认。

## 2026-11-21 — PostgreSQL 订单退款并发幂等测试（PR #65 已合并）

- PR #65 已 Squash Merge 至 main，merge commit `7e2d1f843bbe77bad92252d61b77b6716393dd2f`。
- 新增 `internal/database/orders_refund_postgres_test.go`，使用 CI 临时 PostgreSQL 16 服务验证并发退款同一订单时只有一个成功；钱包余额只增加一次、退款账本只写入一笔、订单最终为 `refunded`，后续重复退款被拒绝。
- PR head Go CI（含 PostgreSQL 测试）、Admin CI、Security 全部成功；本 PR 只增加测试和文档，不修改退款业务逻辑、数据库迁移或生产环境配置。
- 合并后 main Go CI #44783008576、Admin CI #44783008481、Security #44783008445、Docker Publish #44783008524 已触发；Admin/Security 成功，Go/Publish 仍在运行。

## 2026-11-21 — PostgreSQL 订单交付租约并发测试（PR #64 已合并并发布）

- PR #64 已 Squash Merge 至 main，merge commit `1d88c5499d5ca87352391e051fd272cf2772f068`。
- PostgreSQL 16 集成测试覆盖同一订单并发即时租约争抢（单一赢家）及批量领取不同订单（不重复分配）。
- PR head Go CI（含 PostgreSQL 测试）、Admin CI、Security 全部成功；仅增加测试，不改变运行时逻辑、数据库迁移或生产配置。
- main Go CI #44782049335、Admin CI #44782049138、Security #44782049101、Docker Publish #44782049176 全部成功。
- GHCR 多架构 `latest` 与提交标签 `1d88c5499d5ca87352391e051fd272cf2772f068` 已推送；manifest digest：`sha256:a6378128ff6c184dce3c22337df9b4c061f76547fd1d66974f13799d2496570e`。

## 2026-11-20 — PR #61 购物车统一结算合并并发布

- PR #61 已 Squash Merge 至 main，merge commit 59337349d36937a467e673e427f571ec276c17ce；旧 PR #59 已关闭并由该 PR 替代。
- 功能：按 Bot + Telegram 用户隔离的持久化购物车；本地 USDT card/file_card 商品多商品统一结算；事务内重新校验商品、价格、来源、币种、交付配置和库存；预留全部卡密并保存逐商品快照；余额与 TRC20 复用现有单笔订单支付流程；支付后按商品分组交付卡密。
- 私聊边界：购物车回调和 /cart、/cart@bot、回复键盘文本入口均限制在私聊；群聊请求返回提示，不访问购物车数据。
- 数据库迁移：0049_shopping_cart.sql、0050_cart_checkout.sql；随应用启动自动执行，无新增生产环境变量或手工 SQL。
- 验证：PR #61 head Go/Admin/Security 全部成功；main Go CI #44636933083、Admin CI #44636932944、Security #44636932940、Docker Publish #44636932941 全部成功。
- 镜像：GHCR 多架构 latest 与提交标签均已推送；digest sha256:09f9b2c7ee23037ad9c03e26d1b490bdb853988958e6e7804e224bc855039f7d。生产部署与 Telegram 真机验收未确认。

## 2026-11-20 — PostgreSQL 购物车并发集成测试（PR #62 已合并）

- PR #62 已 Squash Merge 至 main，merge commit `d988c6a554392af86ea03b9ba60297e1debab011`。
- Go CI 使用临时 PostgreSQL 16 service；并发集成测试让两个独立 Bot/Telegram 用户同时竞争最后一张卡密，验证且仅有一个 checkout 成功、成功方只预留一张卡密、失败方购物车保留。
- 最新 PR head `50e2136884fdb5bffe0f0e989762f8c973a6c9b9` 的 Go CI、Admin CI、Security 全部成功。
- 不修改运行时业务逻辑、数据库迁移或生产环境配置；`TEST_POSTGRES_DSN` 仅由 CI job 注入临时数据库服务。
- main 合并后 Go CI #44641444572、Admin CI #44641444462、Security #44641444408、Docker Publish #44641444376 已触发；文档提交时 Security 成功，其余仍在运行，镜像 digest 待发布完成后补录。
- Telegram 真机支付/交付验收仍独立待执行；自动化并发测试不替代真实付款、库存和 Bot 交付验收。

## 2026-11-20 — PR E Dujiao-Next 真实商户联调操作手册（PR #58 已合并）

- PR #58 已 Squash Merge 至 main，merge commit 9a4faafd11eaaee0a3d82feb9161ee63d424ad25；独立分支 docs/dujiao-live-integration-pr-e，基线为 PR S merge commit 41389825d6edef83330c4e28c7b5662e23f932a7。
- 新增 docs/dujiao-live-integration-runbook.md：HTTPS 回调与反向代理要求、公网路由验证、回调 HMAC 验签边界、目录同步和来源切换、最低金额真实采购、供应商明确失败 SKU、人工退款/审计、uncertain 人工核实、资金红线及验收记录模板。
- Dujiao-Next 正常对接已由项目所有者确认完成；本手册现仅作为历史排障参考，不再要求执行额外真实商户验收。
- 历史操作手册保留为排障参考；Dujiao-Next 正常对接已由项目所有者确认完成，不作为待办或验收门槛。

## 2026-11-20 — 安全与运维补欠（PR S #57 已合并）

- 基线：PR U 合并后 main commit `b5ada0116fe4c33e0a4207ebb754715834e909d8`；分支 `feat/security-ops-hardening-pr-s`。
- 已开始：核实 PR #53 登出撤销与 TokenVersion 既有实现；增加敏感写 API 限流、可信代理 IP 白名单、详细健康端点、Bot 连续轮询错误告警、Compose 数据库/数据卷备份恢复脚本。
- PR #57 已 Squash Merge 至 main，merge commit 41389825d6edef83330c4e28c7b5662e23f932a7；main Go CI [#44603593840](https://github.com/seanldnet/TG-Card/actions/runs/44603593840)、Admin CI [#44603593868](https://github.com/seanldnet/TG-Card/actions/runs/44603593868)、Security [#44603593955](https://github.com/seanldnet/TG-Card/actions/runs/44603593955) 全部成功；Docker Publish [#44603594008](https://github.com/seanldnet/TG-Card/actions/runs/44603594008) 成功，GHCR 多架构 latest digest `sha256:f9f1dd001bd90c743264d3d16655226626842805f6f8af108c27eb8ff1b639ac`。PR Go CI 包含 Bash 脚本语法检查、go vet、go test 与 CGO_ENABLED=0 构建。
- 边界：Shell 脚本语法已通过 CI；实际 Docker Compose 备份/恢复演练尚未在隔离环境执行。生产部署与验收另行记录。

## 2026-11-20 — 上游目录分页同步、增量处理与安全来源切换（PR U #56 已合并）

- 状态：PR #56 已 Squash Merge 至 main，merge commit `b5ada0116fe4c33e0a4207ebb754715834e909d8`；main Go CI [#44600428873](https://github.com/seanldnet/TG-Card/actions/runs/44600428873)、Admin CI [#44600428589](https://github.com/seanldnet/TG-Card/actions/runs/44600428589)、Security [#44600428737](https://github.com/seanldnet/TG-Card/actions/runs/44600428737)、Docker Publish [#44600428987](https://github.com/seanldnet/TG-Card/actions/runs/44600428987) 全部成功；GHCR 多架构 latest digest `sha256:b92a802644f3fa6a834edf7e7e6d480cdd97419f60242bad3ed63358356afb58`。生产部署与验收仍待项目所有者执行。
- 影响模块：Dujiao-Next 商品目录同步、数据库目录查询、Telegram 商城、上游后台设置、上游目录清理。
- 变更内容：
  - 增加商品分页 API 调用，批次大小/间隔后台可配置；每页独立事务写入，商品指纹结合上游内容和本地价格策略，跳过未变化商品的重复写入/图片下载。
  - 手动/定时同步统一为异步任务，保存进度、错误、状态与取消请求；单页重试、连续失败暂停、Owner 通知、启动时恢复中断任务。
  - Bot 分类/商品/搜索改为来源过滤的限量分页查询，详情按需读取；上游 auto 履约商品进入现有订单/付款/采购队列，manual 履约仍不开放自助购买。
  - 增加停用并清理上游目录的异步确认流程；保留本地商品和历史订单快照，图片删除限定于上游 hash 文件并检查引用。
  - 新增迁移 0044–0048，覆盖同步任务、批次配置、商品/分类指纹和活动任务唯一约束。
- 数据库迁移：0044_upstream_sync_jobs.sql、0045_upstream_sync_batch_config.sql、0046_upstream_product_fingerprint.sql、0047_upstream_sync_single_active_job.sql、`0048_upstream_category_fingerprint.sql`。
- 验证：新增数据库分页/批次/清理快照、商品/分类指纹与任务互斥测试；Go Backend CI [#44598706303](https://github.com/seanldnet/TG-Card/actions/runs/44598706303)、Admin CI [#44598706271](https://github.com/seanldnet/TG-Card/actions/runs/44598706271)、Security [#44598706306](https://github.com/seanldnet/TG-Card/actions/runs/44598706306) 成功；镜像发布待 PR 合并后工作流。
- 兼容性与边界：Dujiao 商品 API 无 updated_since，增量处理仍需扫描分页；分类 API 未确认分页/更新时间。清理仅按 source_type='upstream' 删除。真实环境资源与 Telegram 验收待项目所有者执行。
- 发布镜像：Docker Publish 成功并发布上述 digest；生产部署与数千商品真实压力验收未完成。

## 2026-11-20 — 管理员登出撤销当前 Bearer 会话（PR #53 已合并）

- 修复目标：原 `POST /api/auth/logout` 只返回成功，不撤销服务端 token；被复制的旧 Bearer token 在 24 小时内仍可访问 API。
- 新增 `0041_admin_token_sessions.sql` 与服务端会话登记/查询/撤销；数据库只保存 token SHA-256 摘要。
- 登录登记会话；鉴权检查服务端会话是否仍有效；登出只撤销当前 token，不影响同一管理员的其他设备。密码修改仍由 TokenVersion 撤销全部会话。
- 迁移后既有未登记 token 将失效，管理员需重新登录。
- 新增数据库生命周期、JTI 唯一性与登录/登出集成测试。PR head Go/Admin/Security 全部成功；main Go [#44579627983](https://github.com/seanldnet/TG-Card/actions/runs/44579627983)、Admin [#44579627988](https://github.com/seanldnet/TG-Card/actions/runs/44579627988)、Security [#44579628087](https://github.com/seanldnet/TG-Card/actions/runs/44579628087)、Docker Publish [#44579628246](https://github.com/seanldnet/TG-Card/actions/runs/44579628246) 全部成功；GHCR 多架构 latest manifest digest `sha256:7c14fabea0d23e7903785237052fa69fc1144a8d7be263c420373555131c5e37`。

## 2026-11-20 — Owner Bot 重启恢复、超时与工单切换回归（PR #50 已合并）

- PR #50 已合并至 main，merge commit `eee2b3ae5505d3d97e8a45b07672f53530b78ba9`。
- 新增 C3/C4 自动化测试：Manager 重建后从数据库恢复 reply 会话；TTL 过期后拒绝消费、发送或记录新消息；切换到工单 B 后，A 的旧确认按钮不得结单 A 或影响 B 的活动会话。
- PR head 与 main Go/Admin/Security/Docker Publish 全部成功；GHCR 多架构 latest manifest digest `sha256:eb0776f9dafc7d90aa1928a9f85eeeaf1488891c16ab843826aa0db9ab2a1a4e`。
- 仅测试补强，不改变业务行为、数据库结构或部署配置；真实 Telegram C3/C4 仍待项目所有者验收。

## 2026-11-20 — Owner Bot 继续沟通与媒体回复回归覆盖（PR #47 已合并）

- 新增 C1/C2 自动化测试：Owner 点击“否，继续沟通”恢复 reply 会话；不点按钮连续发送两条消息，两条均进入客户交付与客服时间线。
- 新增图片与普通文件回复测试，验证 Telegram 分别调用 `sendPhoto` / `sendDocument`，发送成功后才保存附件时间线。
- PR #47 已合并至 main，merge commit `7be8ddbb850c907060801dcb69bd0fea9c5e679b`。PR head 与 main Go/Admin/Security/Docker Publish 全部成功；GHCR 多架构 latest manifest digest `sha256:3b763b2410a222c62ed864685ac2cba06c86fbc3942c69aa429d9b702cb02368`。仅测试补强，不改变业务行为、迁移或部署配置。

## 2026-11-20 — Owner 工单确认回调会话隔离加固（PR #44 已合并）

- 状态：已合并至 main，merge commit `0c705ed99c76488aeafa273b019fb3262d19d398`。
- 确认按钮必须匹配当前 Bot、Owner、Telegram chat_id、ticket_id 与 confirm 阶段；旧工单按钮、过期会话及已清理会话均拒绝状态变更。
- Owner 回复会话绑定开始回复时的 Telegram chat_id，其他聊天中的消息不会被当作工单回复。
- 新增迁移 `0040_support_owner_session_chat.sql`；清理无法安全关联 chat_id 的旧会话，Owner 重新打开工单即可。30 分钟 TTL 与重启恢复保持不变。
- 新增双工单旧按钮隔离、重复回调审计幂等、跨聊天会话隔离测试。
- PR head Go/Admin/Security 全部成功；main Go [#44573663656](https://github.com/seanldnet/TG-Card/actions/runs/44573663656)、Admin [#44573663564](https://github.com/seanldnet/TG-Card/actions/runs/44573663564)、Security [#44573663686](https://github.com/seanldnet/TG-Card/actions/runs/44573663686) 全部成功；Docker Publish [#44573663703](https://github.com/seanldnet/TG-Card/actions/runs/44573663703) 成功，GHCR 多架构 latest manifest digest `sha256:64cbbf218f0aff25d0eddf6e784929a134596f1ae7c85b13bec48ba89cf5d1fe`。
- 生产部署与 Telegram 真机 C1–C4 验收待执行。

## 2026-11-20 — 网页后台客服回复送达一致性修复（PR #42 已合并）

- 状态：已合并至 main，merge commit `7fd658d1803d18442d9284098ddf8128cf22cc19`；PR head 与 main Go/Admin/Security 全部成功。Docker Publish Run [#44570069465](https://github.com/seanldnet/TG-Card/actions/runs/44570069465) 成功，GHCR 多架构 latest manifest digest `sha256:025a45cccf1895c17882e863b15bb524adc83200b2a7b77b891cee9e75a9182d`；生产部署/真机验收待确认。
- 影响模块：`cmd/server/support.go`、售后 API、售后后台交互、`.github/workflows/security.yml`、售后专项文档。
- 修复：网页后台文本回复改为先发送 Telegram；明确发送失败时仅记录 system 失败事件，不写入 admin 客服时间线、不更新工单回复字段，返回 502 供前端保留窗口重试。Telegram 明确成功后才写入 admin 时间线并更新工单回复字段；前端状态更新也移至回复发送成功之后，避免发送失败时状态先行变更。
- 测试：新增 Telegram 发送成功/失败回归测试，检查 timeline sender_type、工单状态和 adminReply 字段；PR Go CI、main Go CI、Admin CI 与 Security 均成功。main runs：[Go #44570069329](https://github.com/seanldnet/TG-Card/actions/runs/44570069329)、[Admin #44570069285](https://github.com/seanldnet/TG-Card/actions/runs/44570069285)、[Security #44570069428](https://github.com/seanldnet/TG-Card/actions/runs/44570069428)。
- CI：Security workflow 原先只监听 `internal/**`，会漏掉 `cmd/server/**` 的后端 handler 变更；本次把 `cmd/**` 同时加入 push/PR 路径过滤，确保本类 API 变更触发漏洞扫描。
- 边界：Telegram 无业务消息幂等键；网络超时/响应丢失仍存在远端送达不确定性。发送成功但本地持久化失败时返回明确错误，不声称消息未送达。

## 2026-11-20 — Owner 回复发送状态一致性修复（PR #41 已合并）

- 状态：已合并至 main，merge commit `af53dad378bb8d1d89385246bd7d11a308e8b7e0`。
- 修复 Owner 回复流程：先发送 Telegram，确认成功后才写入客服时间线并切换至待结单确认阶段；发送失败只写系统失败事件并保留 reply 会话。
- 新增 Telegram 成功/失败响应回归测试，覆盖时间线条目数与会话阶段。
- PR head Go/Admin/Security 与 main Go/Admin/Security/Docker Publish 均成功；Docker Publish Run [#44567512730](https://github.com/seanldnet/TG-Card/actions/runs/44567512730) 成功。该轮 GHCR digest 尚未独立核实，生产部署/Telegram 真机验收未确认。
- 边界：Telegram 不提供业务消息幂等键；网络超时/响应丢失仍存在远端送达状态不确定性。

## 2026-11-20 — 售后 Owner Bot 工单列表跨 Bot 隔离修复（PR #40 已合并）

- 修复 `ListPendingSupportTickets` 未按 bot_id 过滤的跨 Bot 工单可见性问题。
- 数据库查询现在要求非空 Bot ID，并在 SQL 条件中同时限定 bot_id 与待处理状态；Owner Bot 列表使用当前 worker 的 Bot ID；工单查看、回复、结单和确认结单回调均校验工单归属当前 Bot。
- 新增多 Bot 隔离回归测试，覆盖 Bot A 只能看到 A 工单、Bot B 只能看到 B 工单，以及空 Bot ID 失败关闭；新增回调层跨 Bot 工单访问拒绝测试。
- 不改变已由项目所有者确认通过的附件转发行为。
- PR #40 merge commit `69218784601f7d75b668b8047379d931c9a87564`；PR head Go/Admin/Security 全部成功，main Go/Admin/Security/Publish 全部成功。
- GHCR `latest` digest：`sha256:0a0ca07e37b42b46b6c6bfb6cc6a2519348066fc6ae62bf98eb4c5b09134b2d7`；Publish Run [#44565112822](https://github.com/seanldnet/TG-Card/actions/runs/44565112822)。生产部署/真机验收待确认。


## 2026-11-20 — 阶段一售后附件转发子项所有者实测确认

- 项目所有者确认：售后附件转发相关测试已亲自完成且无异常；将“用户提交图片/文件后由对应 Bot 转发给 Owner，Owner 收到原始媒体”这一子项记录为所有者实测通过。
- 证据边界：本次未提供测试镜像 digest、设备、订单/工单号或截图；不将本次确认绑定到 PR #38 镜像，也不补造测试证据。
- PR #27 C1–C4 其余交互验收、菜单全链路、补发/退款全分支、CDN Network 检查仍待完成；阶段一尚未清零。
- 同步纠正 PR #38 状态：已合并并发布，merge commit `935bd222977b1a636c5f05ab4faebdc3e835dda1`，GHCR digest `sha256:691493ce844c96a504785e0fb72b24534786f57279190322332f45b5ba8b7b29`；项目所有者已确认 Dujiao-Next 正常对接完成，不再追加 PR E 真实商户验收。
- 本次为验收记录修订，不代表阶段完成；文档版本号待阶段一全部完成后按强制规则递增。

## 2026-11-19 — Dujiao-Next PR D 资金安全深化（PR #38 已合并并发布）

- PR：[PR #38](https://github.com/seanldnet/TG-Card/pull/38)，独立分支 feat/dujiao-fund-safety-pr-d，head 5dbd0269a719f91b74458f03bd67a25aeab0761b，基线 main@0b0f0514a1c5850aafec0018c95570d0bce8a39f。
- 状态：PR #38 已合并至 main，merge commit `935bd222977b1a636c5f05ab4faebdc3e835dda1`；main Go/Admin/Security/Docker Publish 均成功；GHCR digest `sha256:691493ce844c96a504785e0fb72b24534786f57279190322332f45b5ba8b7b29`。生产部署/验收待项目所有者确认。
- 实现：供应商采购前后余额快照和差异异常、15 分钟未完成快照巡检、退款账本完整性检查与重复退款保护、uncertain ageing/Owner 核实恢复、统一资金异常中心及处理状态。
- 测试：go vet ./...、go test ./...、CGO_ENABLED=0 go build 全部成功；SQLite 测试覆盖供应商资金差异、退款关联异常不自动修正、重复退款仅一笔 credit、uncertain ageing 和 Owner 恢复。
- 资金边界：不自动修改买家或供应商余额；采购失败/重试不改原始支付引用；退款单订单唯一；供应商差异只进入人工审查。Dujiao-Next 正常对接已由项目所有者确认完成；不再追加供应商扣款演练。

## 2026-11-19 — Dujiao-Next PR D 资金安全深化（历史开发记录，已由 PR #38 合并完成）

- 历史状态：该分支开发记录已由后续 PR #38 合并与发布结果取代；当前以本文件上方“PR #38 已合并并发布”记录为准。
- 仓库审查：PR2 只读校验覆盖买家余额流水；PR C 退款有唯一退款行与唯一钱包流水关联，但缺少完整性巡检；供应商余额快照尚未按采购逐笔与扣款金额比对；uncertain 缺少 ageing/独立人工核实；无统一资金异常中心。
- 实际合并内容：新增迁移 0039_finance_safety.sql、供应商采购前后余额对照、退款账本一致性检查、uncertain ageing 与 Owner 核实 API、统一资金异常 API/后台页面和定时巡检。检测只生成异常与通知，不自动调整余额。
- 新增 SQLite 测试覆盖供应商余额差异、退款账本损坏检测、重复退款拒绝、uncertain ageing/Owner 恢复；需以 PR Actions 最终结果为准。
- 资金红线：退款仅一条独立账本并关联唯一 wallet credit；采购重试沿用原本地订单幂等键；采购失败不改变原始支付记录；不自动修改买家或供应商余额。
- Dujiao-Next 正常对接已由项目所有者确认完成；PR #27 Owner Bot C1–C4 已由项目所有者确认测试通过（2026-11-22）。

## 2026-11-19 — PR B 上游商品同步验收通过

- 项目所有者确认后台绑定商户、目录同步、加价展示和来源切换均正常；PR B（PR #26）D1–D5 验收状态更新为已验收，日期 2026-11-19。
- 验收镜像仍为生产 PR2 digest sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e。PR #27 Owner Bot C1–C4 已由项目所有者确认测试通过（2026-11-22）。

## 2026-11-18 — Dujiao-Next PR C 采购转发（PR #32 已合并，CI/Mock 通过）

- 状态：独立 PR #32 已合并，merge commit `4f712e6ecb2299abd1eb782a439dee56b760578c`；PR head Go/Admin/Security 与 main Go/Admin/Security/Publish 全部成功；十项 mock 测试通过。项目所有者已确认 Dujiao-Next 正常对接完成；该确认不改变 PR #32 mock 测试的实际覆盖范围。
- 分支：`feat/dujiao-purchase-forwarding`，基线 `main@2e7f91cfe772c3a87b3c0c40d8542bff59b9c9be`。
- 实现：支付确认事务登记采购任务；采购前复核订单/商品；本地订单号幂等创建、上游订单 ID/订单号映射；查单轮询、回调验签/幂等；加密交付；失败人工队列、手动恢复、站内余额退款独立账本与审计；供应商余额快照/阈值告警/日报。
- 资金安全：采购失败和重试不修改原支付记录、不自动退款买家余额；管理员手动退款使用独立钱包 credit 流水和退款审计。
- 协议限制：官方 API 未提供按 `downstream_order_no` 查单接口。首次 POST 网络异常且未获得上游订单 ID 时进入 `uncertain`，暂停自动重发；人工恢复仍使用原幂等键。已获得上游订单 ID 时才调用 `GET /orders/:id`。
- 当前限制：后台必须配置公网 HTTPS 回调地址才允许上游自助下单；只开放 auto 自动交付 SKU，manual 表单 SKU 暂不开放。Dujiao-Next 正常对接已由项目所有者确认完成，不再要求额外 PR E 验收。
- Mock 测试：同步交付、异步查单成功、查单超时、采购失败、重复回调、验签失败、超时后同幂等键恢复、已退款订单跳过、HTTP 402/404 错误分类共 10 项通过。
- PR：[PR #32](https://github.com/seanldnet/TG-Card/pull/32)。PR #32 merge Publish digest：`sha256:206c0ca3a12526aaeeb6ec85e17adbbc155803a8f3dc64d9f76667ac8f1fa0cf`；main 重建 Publish Run #44338105044 成功，当前 latest digest：`sha256:1228783d3d8c9c7fab302cc86f60a17866c783d52cf26f27000bcc56e2a55be8`。PR2 生产验收记录保留；PR D 已合并发布，Dujiao-Next 正常对接已由项目所有者确认完成。

## 2026-11-19 — PR2 生产验收通过；Dujiao-Next PR C 已合并

- 状态：PR #30（PR2）已合并至 main；Go CI、Admin CI、安全扫描、Docker Publish 全部成功。项目所有者确认生产已更新至 PR2 镜像，PR #19、#26、#28 与 PR2 验收按默认通过处理，验收日期 2026-11-19。
- PR2 merge commit：`75a635a6c5e9c68e8fc23c229f932119dca46a93`；Publish Run [#44324284760](https://github.com/seanldnet/TG-Card/actions/runs/44324284760)；GHCR multi-platform manifest digest：`sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e`。
- Go CI [#44324284650](https://github.com/seanldnet/TG-Card/actions/runs/44324284650)、Admin CI [#44324284642](https://github.com/seanldnet/TG-Card/actions/runs/44324284642)、Security [#44324284777](https://github.com/seanldnet/TG-Card/actions/runs/44324284777) 均成功。
- PR #19/#26/#28 与 PR2 的本次验收项均使用生产 PR2 digest `sha256:bca6cc3c12df3bd667c8ee269f0ab9794d39a6dda47d1d185ad0894203c4796e` 记录为已验收。该阶段未覆盖 PR #27 Owner Bot C1–C4；项目所有者已于 2026-11-22 后续确认 C1–C4 全部验收通过。
- 采购转发、供应商扣款、异步查单/回调、失败人工队列、退款审计现进入独立 PR C；PR D 资金安全已合并；Dujiao-Next 正常对接已由项目所有者确认完成。

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
- 后续顺序：文档修正 → PR2 资金与库存安全 → Dujiao-Next PR C 采购转发 → PR D 资金安全 → Dujiao-Next 正常对接 → PR3 异常中心 → PR4 购买体验 → PR5 数据看板 → PR6 群发精细化。
- PR 描述要求：每个 PR 写明实现范围、复现条件、验证结果和文档更新；每批独立分支，CI 通过并合并后再启动下一批。


## 2026-11-18 — 售后工单改为 Bot 转发 Owner 与交互式回复（独立 PR #27）

- 状态：已合并至 main（PR #27）；Go CI、Admin CI、安全扫描成功；Docker latest 已推送（Publish Run `44308515206`，digest `sha256:78b216140715964bf201f398e492c8f636091525a1ca327bcfaaa426532bfba2`）。Telegram/SMTP 实际投递已由项目所有者确认测试通过；后续 PR #28 最新生产验收镜像 digest 见本文顶部记录。
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

- 历史状态：当时测试夹具已修正并等待 Go CI 复验，PR #26 尚未合并；后续合并状态以 PROJECT_STATUS.md 第 19 节为准。
- Commit：`4575745f6f35ab0a1ebcdcf36257f63be4c034db`（测试文件提交；文档提交随后更新）。
- 根因：`SaveUpstreamSettings` 持久化调用方传入的 `CatalogStale`，不会自行推断配置是否变更；实际配置 API 在配置变更时负责将该标志置为 true。测试夹具只设置了 `Enabled=true`，却假定数据库层会自动设置 stale，导致测试误走“上游已启用且目录新鲜”的分支，空的上游活跃商品列表被错误地拿来断言本地目录。
- 修复：测试显式设置 `CatalogStale:true`，使其符合配置 API 的真实调用契约；随后仍要求成功同步后才切换到上游目录。
- 验证：本次修正尚未由 Actions 验证；不得据此宣称 Go test 通过。

## 2026-11-18 — 修正 PR #26 上游目录切换测试（配置变更后必须先同步）

- 历史状态：当时测试修正已提交并等待 Go CI 复验；后续合并状态以对应 PR 记录为准。
- PR：https://github.com/seanldnet/TG-Card/pull/26
- Commit：`f46a33296ef83c8619abfb7be5edc8225a0e3a28`
- 根因：上一轮 Go test 仍按“仅保存 Enabled=true 即立即展示上游目录”的旧假设编写断言，与当前安全策略不符。配置变化会将目录标记为 stale，必须成功完成一次同步后才能切换到上游目录。
- 修复：测试先确认 stale 状态仍展示本地目录，再执行成功同步并确认切换到上游目录，随后验证成功的空目录同步会隐藏已不再存在的上游商品。
- 验证：修复后的 Actions 尚待完成；不把提交或测试断言当作通过证据。

## 2026-11-18 — 修复 Dujiao-Next 分类图标同步回归（PR #26）

- 历史状态：当时修复已提交并等待 PR CI 复验；后续合并状态以对应 PR 记录为准。
- PR：https://github.com/seanldnet/TG-Card/pull/26
- 修复 commit：`c402bc8345fb6d2cfc33350457e8083e87fe0948`
- 根因：分类图标更新 SQL 的 CASE 表达式占位符顺序与参数顺序不一致；当图片下载失败、要求保留旧图标时，旧逻辑把布尔值写入 `upstream_icon`，导致测试失败。
- 修复：按 SQL 占位符顺序传入 `PreserveIcon` 与新图标值；保留既有图标的分支不再写入布尔值。
- 验证：上一轮 Go vet 通过；Go test 的 `TestUpstreamCatalogSyncIsTransactionalAndSourceAware` 暴露此问题。修复后的 Go test/build 与完整 CI 尚待新一轮 Actions 结果确认。
- 资金安全：本修复不涉及订单、买家扣款、供应商采购或退款逻辑。

## 2026-11-18 — Dujiao-Next PR B：商品目录同步与来源管理

- 状态：PR #26 已合并至 main；Go CI、Admin CI、安全扫描及 Docker Publish 均成功；Dujiao-Next 正常对接已由项目所有者确认完成。
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
- 兼容性与已知限制：当前订单结算为 USDT；非 USDT 上游币种必须人工配置“1 单位上游币种 = ? USDT”的汇率。每个 SKU 单独映射为商品；最多下载每商品前 5 张图片。该阶段上游采购/付款转发尚未实现；后续已由 PR C（PR #32）实现并合并，资金安全深化由 PR D（PR #38）实现并发布。
- 验证：
  - 新增官方响应 JSON 结构、价格解析/汇率/加价、SKU 规格名称单元测试。
  - 新增 SQLite 源切换、事务回滚、缺失商品隐藏、图片失败保留、stale 标志和设置持久化测试。
  - PR #26 Go CI、Admin CI、安全扫描均成功；Dujiao-Next 正常对接已由项目所有者确认完成。
  - 官方协议依据：`docs/upstream-integration.md` 所列 Dujiao-Next Open API 文档。
- 发布镜像：PR #26 已发布；Publish Run #44299965343 成功，digest `sha256:c805ce4a8aa2ee6f2adcf01d16122d15117c9e813457a740c00f2d31e7ec66ae`；后续 latest 已由 PR #27/#28 更新。

## 2026-11-18 — Dujiao-Next PR A：上游基础设施（配置与连接测试）

- 状态：PR #21 已合并至 main；Go CI、Admin CI、安全扫描均通过；正常对接已确认完成。
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
- 兼容性与已知限制：PR A 不执行目录同步、不改变本地商品展示、不采购、不扣上游余额；同步状态字段仅用于后续 PR B。Dujiao-Next 正常对接已由项目所有者确认完成。
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

- 历史状态：该 PR 合并并发布时，真实环境验收尚未完成；项目所有者随后于 2026-11-19 确认生产更新及对应验收通过，详见 PROJECT_STATUS.md 第 19.5 节。
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
