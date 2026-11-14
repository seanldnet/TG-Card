# TG-Card

单商户、多 Telegram Bot 的自动发卡系统。

## 技术栈

- Admin UI: Vue Vben Admin 5.x（Vue 3 + TypeScript + Vite）
- Backend: Go
- Database: Docker 默认 PostgreSQL；独立二进制支持 SQLite / PostgreSQL
- Deployment: Docker Compose（预构建镜像，无需在服务器编译）

## 产品边界

- 单商户、单管理员体系；支持添加和管理多个 Telegram Bot。
- 多个 Bot 共用商品、卡密库存、订单和支付业务核心。
- 每个 Bot 可单独配置 Token、名称、欢迎语、菜单和商品展示。
- 不包含多商户、代理商或分销体系。

## 计划模块

仪表盘、机器人管理、商品分类、商品管理、卡密库存、订单管理、支付渠道、用户管理、财务统计、系统设置、管理员与操作日志。

## 一键部署目标

最终发布后，服务器只需准备 Docker 与 Docker Compose，下载 `docker-compose.yml`，执行：

```bash
docker compose up -d
```

然后访问 `http://服务器IP:38647`。数据库迁移、初始管理员创建和静态前端资源由容器自动完成。正式发布前会提供安全的首次初始化流程，不使用公开固定密码。

## 当前状态

项目正在初始化。后续提交会逐步交付 Go API、Vben 管理界面、Telegram Bot 多实例运行时、支付和自动发卡流程，以及 GHCR 自动构建发布。

## 功能范围

完整功能清单与实施优先级见 [`docs/product-scope.md`](docs/product-scope.md)，覆盖商品与卡密、Bot 欢迎页和动态菜单、订单支付自动交付、售后工单、签到奖励、余额积分、优惠券活动、USDT 链上收款、GMpay、EPUSDT，以及后台运营与安全管理。
