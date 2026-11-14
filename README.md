# TG-Card

单商户、多 Telegram Bot 的自动发卡商城系统。

## 技术栈与架构

- Admin UI：官方 Vue Vben Admin 5.x，使用 apps/web-antd（Vue 3 + TypeScript + Vite + Ant Design Vue）
- Backend：Go HTTP API
- Database：Docker 默认 PostgreSQL；独立运行支持 SQLite / PostgreSQL
- Authentication：Vben 官方请求层与认证状态，Go 后端签发 HS256 Bearer Token
- Deployment：多阶段 Docker 构建，GHCR 预构建镜像（linux/amd64、linux/arm64）

## 产品边界

- 单商户、单管理员体系；支持添加和管理多个 Telegram Bot。
- 多个 Bot 共用商品、卡密库存、订单和支付业务核心。
- 每个 Bot 可单独配置 Token、名称、欢迎语、菜单和商品展示。
- 不包含多商户、代理商或分销体系。

## 产品介绍首页与管理后台

- 产品介绍首页：/，也可访问 /intro。
- 管理后台登录：/auth/login。
- Vben 上游源码通过 Git Submodule 固定版本引入；TG-Card 的页面、路由和构建配置放在 web/overrides/，不直接修改上游子模块。
- 首次克隆：git clone --recurse-submodules https://github.com/seanldnet/TG-Card.git。
- 已有克隆：git submodule update --init --recursive。
- 本地构建前端：./scripts/build-admin.sh。构建产物会复制到 web/dist，由 Go 服务托管。
- API 地址与前端同源，统一使用 /api 前缀；Vben 使用 History 路由，Go 服务提供 SPA fallback。

## 第二阶段已交付的基础能力

- SQLite / PostgreSQL 数据库连接与启动检查。
- SQL 文件版本化迁移，迁移版本记录在 schema_migrations。
- 首次启动自动初始化超级管理员，密码使用 bcrypt 哈希。
- 未设置 ADMIN_PASSWORD 时生成一次性随机密码，仅在首次启动日志显示；请立即保存并妥善保管。
- HS256 登录令牌；未设置 JWT_SECRET 时，在持久化的 DATA_DIR/jwt-secret 中自动生成签名密钥。
- 登录、用户信息、权限码、菜单接口遵循 Vben 的 { code, data, error, message } 响应结构。
- 登录成功/失败审计记录。
- Go 服务托管 Vben 静态资源并支持前端 History 路由回退。
- GitHub Actions 提供 Go CI 和 GHCR 多架构镜像构建发布。
- TRON 主网 USDT 收款监听：在管理后台「支付设置」中填写 TronGrid API Key、收款地址并启用后，Bot 为每笔订单显示唯一六位小数应付金额并监听已确认转账；配置加密保存在数据库，无需编辑 .env 或重启容器。

## 本地运行

需要 Go 1.23+。默认 SQLite，无需单独安装数据库：

~~~bash
go mod download
go run ./cmd/server
~~~

首次启动会在日志中打印随机管理员密码。默认监听 38647，数据库和签名密钥位于 ./data。

构建并托管前端：

~~~bash
git submodule update --init --recursive
./scripts/build-admin.sh
go run ./cmd/server
~~~

PostgreSQL 可通过环境变量启用：

~~~bash
DB_DRIVER=postgres
DATABASE_URL=postgres://user:password@127.0.0.1:5432/tgcard?sslmode=disable
ADMIN_USERNAME=admin
ADMIN_PASSWORD='replace-with-at-least-12-characters'
~~~

## Docker 部署

准备 Docker Compose 和 .env（从 .env.example 复制并设置强 POSTGRES_PASSWORD），然后：

~~~bash
docker compose up -d
~~~

容器使用 GHCR 的 latest 镜像，默认端口为 38647。首次启动时查看容器日志获取自动生成的管理员密码：

~~~bash
docker compose logs app
~~~

数据持久化在 Docker volumes 中。不要删除数据卷，否则会丢失数据库和 JWT 签名密钥。

## API 基础端点

- GET /api/health：健康检查。
- POST /api/auth/login：管理员登录。
- POST /api/auth/logout：退出登录（当前 Bearer Token 为短期无状态令牌，客户端清除令牌）。
- GET /api/user/info：当前管理员信息。
- GET /api/auth/codes：当前管理员权限码。
- GET /api/menu/all：当前管理员可见的动态菜单。

## 产品功能范围

完整功能清单与实施优先级见 docs/product-scope.md。TRC20 当前使用单一收款地址与唯一金额识别，晚到/少付/多付交易的人工对账、交易流水后台、退款和钱包归集仍待开发；GMpay、EPUSDT 也尚未接入。支付凭据在管理后台设置并加密存储，不需要写入部署环境变量。基础认证和数据库已完成不代表完整商城已交付。
