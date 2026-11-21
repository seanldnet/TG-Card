## 管理员敏感操作二次确认（PR #100 已合并）

- 目标操作必须先通过管理员当前密码再次认证；若管理员启用 TOTP，还必须提交有效动态码或消费一枚一次性恢复码。
- 服务端签发 2 分钟有效的一次性随机凭证，数据库只保存 SHA-256 哈希；凭证绑定管理员、action key、HTTP method、精确 escaped path 与规范化 JSON body。
- 目标 API 使用原子条件更新消费凭证，防止重复提交、跨管理员使用、跨路径/参数复用及并发重放。
- 前端收到 HTTP 428 challenge 后弹出统一密码/动态码对话框，确认成功后仅重放原始请求；二次确认失败或取消不会执行目标操作。
- 覆盖余额变更、采购重试/退款/状态核实、支付人工核对、售后退款、订单交付恢复/步骤核实、订单清理、支付/站点/上游/充值赠送配置及上游数据清理。
- 新增 migration 0057；不存储密码、TOTP 明文、恢复码明文或请求体内容；审计仅记录 action key 与目标路径。
- PR #100 已 Squash Merge，merge commit `50ecad1ca9fd72ec319e46b1d26fd659a3ad8f34`；PR head 与合并后 Go/Admin/Security CI 全部通过。Docker Publish run `44885024397` 已成功；GHCR manifest digest：`sha256:1ef5ec7d1ef7cc75b9b764bcbea972b83687798468f705e0a4da3664704493a2`。真实浏览器 step-up 流程待部署验收。

## 管理员登出 Access Session 即时撤销补强（PR #99 已合并）

- Vben Admin 上游 `logoutApi` 使用不附加 Authorization 的 baseRequestClient；仅靠 refresh cookie 登出时，服务端无法定位并撤销当前 access session。
- 本项目覆盖 Vben `apps/web-antd/src/api/core/auth.ts`，登出改用认证 requestClient，并保留 withCredentials，使请求同时携带 Bearer 与 HttpOnly refresh cookie。
- 服务端按当前 Bearer 撤销 access session，并按 cookie 撤销 refresh token family；新增 HTTP 回归测试验证旧 Bearer 立即被拒绝。
- 无迁移、无新增配置；真实浏览器验收仍需执行。

## 管理员短效 Access Token 与 Refresh Token 轮换（PR #97 已合并）

- Access Token 15 分钟；Refresh Token 7 天，HttpOnly、SameSite=Lax Cookie，Path 限定为 `/api/auth`。
- Refresh token 以 SHA-256 哈希保存；每次刷新旧 token 原子作废并创建新 token，同时创建短效 access session。
- 已撤销 token 再次使用视为重放，撤销同一 family 下全部 refresh token，并写管理员审计和全局异常。
- 密码/TOTP 修改导致 token_version 递增，refresh 时校验版本，旧 refresh family 随即失效。
- 登出接口兼容 Vben Admin 通过 `withCredentials` 发送 refresh cookie 的调用方式。

## TOTP 一次性恢复码（PR #96 已合并）

- TOTP 绑定确认时生成 10 个 80-bit 随机 Base32 恢复码；每个仅可使用一次。
- 恢复码明文只在启用成功响应中返回一次，响应设置 `Cache-Control: no-store`；数据库仅保存 SHA-256 哈希，不保存明文。
- 恢复码消费通过旧哈希列表条件更新（compare-and-swap）防止并发双重使用；成功使用写入管理员审计。
- 关闭 TOTP 时清空全部恢复码；丢失验证器时可用一枚恢复码登录后重新配置验证器。
- 用户必须在绑定完成页面复制并安全保存；离开页面后无法再次查看。剩余恢复码数量当前不向页面暴露。

## 管理员 TOTP 双重验证（P1，PR #95 已合并）

### 设计

- 管理员可在“安全设置”中启用 TOTP（RFC 6238，HMAC-SHA1、6 位、30 秒周期，允许前后各一个时间步偏差）。
- 启用绑定需要再次输入当前密码；服务端生成 160-bit 随机 Base32 密钥，使用现有 AES-GCM Secret Manager 加密后保存，数据库不存明文密钥。
- 绑定过程先保存加密 pending secret；管理员使用验证器应用输入有效动态码后，才原子启用 TOTP。取消或未完成绑定不会影响当前密码登录。
- 已启用 TOTP 的管理员登录必须同时提供密码、Cap（若已启用）和动态验证码；密钥解密失败时 fail closed。
- 关闭 TOTP 必须再次验证当前密码和动态验证码；绑定、启用、关闭均写入管理员审计日志，相关接口有独立限流。
- 登录页面增加可选动态验证码字段；未启用 TOTP 的管理员不受影响。

### 迁移与验证

- 迁移 0054_admin_totp.sql 为 admins 增加加密 active/pending secret、enabled 标志和启用时间；不新增环境变量。
- 新增 TOTP 算法与 provisioning URI 单元测试、管理员 TOTP 状态持久化测试。
- 绑定密钥只在初始化时显示一次；请在启用前先保存至验证器应用。PR #96 补充一次性恢复码，恢复码也只显示一次；生产启用前仍需完成真实绑定和恢复码登录验收。
- PR #95 已合并；Go/Admin/Security/Docker Publish 均通过，GHCR digest：`sha256:b51ec8ed881cf73d52e2038e6f68a3064b3c818b0e59132970df2339b77fa17a`。真实管理员绑定、动态码登录与恢复码登录仍待生产验收。

# 安全加固记录

## 管理员 Bearer Token 会话撤销（PR #53 已合并）

### 风险

原 `POST /api/auth/logout` 只返回成功，未在服务端撤销签名 token。客户端即使清除本地 token，任何此前复制的 Bearer token 仍可在其 24 小时有效期内继续访问 API。

### 设计

- 新增 `admin_token_sessions` 会话登记表，保存 token 的 SHA-256 摘要、管理员 ID、过期 Unix 时间和创建时间；不保存原始 Bearer token。
- 每个 token 在签名载荷中包含独立随机 JTI，确保同一秒内多次登录也会签发不同 token；登录后先将摘要登记入数据库，再向客户端返回 token。
- 每个鉴权请求除验证 HMAC 签名、管理员状态和 TokenVersion 外，还必须命中未过期的服务端会话记录。
- 登出只删除当前 Bearer token 对应的会话，不影响同一管理员在其他设备的有效会话。
- 修改密码仍通过递增 TokenVersion 撤销该管理员全部旧 token。
- 登录时清理已过期会话记录，避免表无限增长。

### 迁移与升级影响

迁移 `0041_admin_token_sessions.sql` 新建会话表。升级后，迁移前已签发但未登记的 token 会被拒绝；管理员需要重新登录一次。无需新增环境变量或人工数据库操作，应用启动时自动迁移。

### 验证

- 数据库测试：登记、查询有效会话、撤销当前 token、保留另一设备 token、过期拒绝。
- Token 单元测试：验证每次签发生成不同 JTI，避免同秒登录生成相同 bearer token。
- HTTP 集成测试：登录两次、登出其中一个 token、确认该 token 被拒绝而另一个仍有效。
- PR #53 已合并至 main，merge commit `8260a9c4775726587c33100c939eb78018de8cb3`；PR head 与 main Go/Admin/Security/Docker Publish 全部成功。GHCR 多架构 latest manifest digest `sha256:7c14fabea0d23e7903785237052fa69fc1144a8d7be263c420373555131c5e37`。
- 生产部署和真实登录/登出验收未完成前，不标记生产安全验收通过。

### 后续安全事项

- 评估跨实例/反向代理场景下的持久化登录限流与全 API 限流。
- 评估反向代理部署时的监听地址、可信代理 IP 与安全响应头策略。
- 目前管理后台使用 Authorization Bearer 而非 Cookie；不要在未核实前声称存在或需要 Cookie CSRF token。跨源浏览器请求因缺少 CORS 授权默认不可读。
