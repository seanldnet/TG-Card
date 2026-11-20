# Cap 人机验证配置与应急操作

## 功能范围

管理员可在「设置 → 网站设置 → 人机验证」中配置：

- 服务域名：Cap 服务的 HTTPS origin，例如 `https://cap.example.com`，不要填写路径。
- Site Key：Cap 站点公钥。
- Secret Key：Cap 站点私钥。后端使用现有 AES-256-GCM 密钥加密后写入 PostgreSQL 的 `settings` 表；读取接口只返回是否已配置，不返回密文或明文。

启用后，管理员登录页从 Cap 服务的 `/assets/widget.js` 加载 `<cap-widget>`，widget 完成挑战后把一次性 token 随登录请求提交给后端。后端调用：

```text
POST {CapDomain}/{SiteKey}/siteverify
Content-Type: application/json

{"secret":"<Secret Key>","response":"<widget token>"}
```

只有 HTTP 成功且响应 JSON 中 `success` 严格为 `true` 才允许继续校验管理员账号密码。token 缺失、过期、重复使用、服务不可达或响应无效都会拒绝登录。Secret Key 不会下发给浏览器。

## Cap 服务端准备

Cap 自托管实例需要提供站点挑战 API，并启用静态资源服务，使浏览器可访问：

- `https://cap.example.com/assets/widget.js`
- `https://cap.example.com/{site-key}/`

若 Cap 服务没有启用 assets server，前端 widget 无法加载；请先在 Cap 服务配置中启用静态资源服务，或将官方 widget 资源部署到该域名的 `/assets/widget.js`。

浏览器必须能访问 Cap 域名；TG-Card 应用容器也必须能通过 HTTPS 访问同一域名的 `/{site-key}/siteverify`。请在开启验证前完成两侧连通性检查。

## 开启步骤

1. 登录 TG-Card 管理后台，打开「设置 → 网站设置 → 人机验证」。
2. 填写 HTTPS 服务域名、Site Key、Secret Key。
3. 点击保存，确认 Secret Key 显示为“已加密保存”。
4. 开启“启用登录人机验证”，再次保存。
5. 退出管理后台，打开登录页，确认 widget 能正常显示并完成挑战。
6. 使用正确和错误管理员密码分别验证：有效 token 才能进入账号密码校验；缺失/错误 token 必须被拒绝。

## 紧急关闭

**优先使用数据库开关**，无需修改容器环境：

```bash
docker compose exec db psql -U tgcard -d tgcard -c "UPDATE settings SET value='false' WHERE key='cap_enabled';"
docker compose restart app
```

应用读取数据库开关，不需要删除 Cap 域名或密钥。修复 Cap 服务后，可在后台重新开启。

也可使用环境变量覆盖数据库开关：

```bash
# 在 .env 中设置
DISABLE_CAP=true
docker compose up -d --force-recreate app
```

Compose 会将 `DISABLE_CAP` 注入 app 容器。环境变量方式需要重建容器以加载新环境，单纯 `docker compose restart app` 不会更新容器环境。恢复时将其改为 `false` 或移除，再执行 `docker compose up -d --force-recreate app`。

## 数据库与密钥

迁移 `0051_system_settings.sql` 创建通用键值设置表。Cap 配置键：

- `cap_enabled`：字符串 `true` / `false`
- `cap_domain`：HTTPS origin
- `cap_site_key`：公开 Site Key
- `cap_secret_ciphertext`：AES-256-GCM 密文

数据库备份与 `tg_card_data` 数据卷必须一并保护。密文依赖持久化的 `DATA_DIR/bot-token.key` 解密；仅恢复 PostgreSQL 而丢失该密钥文件，会导致 Cap Secret 无法解密。不要把 Secret Key、密文密钥文件或数据库备份提交到 Git。

## 安全与限制

- 服务域名只接受 HTTPS origin；拒绝 localhost、私有/本地 IP、URL 路径、查询参数和片段，避免把管理员配置变成明显的内网请求代理。
- 后端禁用 HTTP 重定向跟随，验证请求有 5 秒超时，响应体最多读取 64 KiB。
- 登录验证失败采用 fail-closed：Cap 已启用但验证服务异常时不会放行登录。
- 当前保护范围是管理员登录接口；其他敏感表单可在后续阶段复用同一 token 验证器。
- `DISABLE_CAP=true` 是最高优先级紧急覆盖，不会改写数据库中保存的启用状态。

## 全局异常中心联动

- Cap 配置读取失败、Secret Key 解密失败会分别记录为 \`cap_configuration\`；验证 token 被拒绝或验证服务异常记录为 \`cap_verification\`。
- 同一类配置故障使用稳定来源键聚合；验证失败按管理员用户名聚合，重复事件更新同一异常记录，避免每次登录失败都无限新增行。
- 异常详情不会保存 Secret Key、密文或一次性 widget token。管理员可在“全局异常中心”查看原因；修复 Cap 服务/配置后再标记异常解决。异常中心不会自动关闭 Cap，也不会绕过 fail-closed 验证。
- 详见 `docs/system-exceptions.md`。
