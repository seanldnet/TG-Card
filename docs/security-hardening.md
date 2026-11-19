# 安全加固记录

## 管理员 Bearer Token 会话撤销（PR #53 开发中）

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
- Go/Admin/Security CI 与 Docker 发布状态以 PR #53 和主分支 Actions 为准。
- 生产部署和真实登录/登出验收未完成前，不标记生产安全验收通过。

### 后续安全事项

- 评估跨实例/反向代理场景下的持久化登录限流与全 API 限流。
- 评估反向代理部署时的监听地址、可信代理 IP 与安全响应头策略。
- 目前管理后台使用 Authorization Bearer 而非 Cookie；不要在未核实前声称存在或需要 Cookie CSRF token。跨源浏览器请求因缺少 CORS 授权默认不可读。
