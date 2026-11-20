# 全局异常中心

## 目标与数据边界

\`system_exceptions\` 是全局运行异常的统一索引，聚合采购/资金异常、订单交付不确定、退款冲突、上游目录同步失败和 Cap 验证/配置异常。它不替代业务原始记录：订单、交付事件、采购事件、同步任务和管理员审计仍保留在各自业务表中。

- \`source_key\` 是异常来源的幂等键。重复报告同一来源会更新原记录，而不是无限创建重复行。
- 来源再次发生时，如果原异常已解决，会重新打开并清空旧解决信息；仍在处理中则保留处理状态并刷新详情。
- 原 \`finance_exceptions\` 数据通过迁移 0052 一次性复制，保留原 ID、来源键、状态、处理说明、处理人和时间戳。旧表保留用于兼容/回滚，不再作为新异常写入的主表。
- 标记解决/忽略只改变异常状态，不会修改订单、库存、钱包、采购余额或支付流水。
- 状态更新和 \`audit_logs\` 操作审计在同一事务提交；审计写入失败时，异常状态更新一并回滚。

## 异常分类

| category | 来源 | 说明 |
|---|---|---|
| \`delivery_uncertain\` | 订单交付状态机 | Telegram 远端副作用结果无法确认，订单进入人工核实队列 |
| \`refund_conflict\` | 售后退款 | 退款与活动中的订单再次获取租约冲突 |
| \`procurement_failed\` | 上游采购 | 采购明确失败 |
| \`procurement_uncertain\` | 上游采购 | 请求已发出但上游受理结果不确定 |
| \`supplier_balance\` | 供应商资金核对 | 采购前后余额快照不匹配或无法核实 |
| \`refund_ledger\` | 退款账本巡检 | 订单、退款记录和钱包 credit 流水关联不一致 |
| \`upstream_sync\` | 上游目录同步 | 同步任务失败 |
| \`cap_verification\` | 管理员登录 | Cap token 被拒绝、过期或验证服务异常 |
| \`cap_configuration\` | 管理员登录 | Cap 配置读取或密钥解密失败 |

异常详情不得包含 Bot Token、Cap Secret Key、JWT、支付 API Key 或一次性 Cap token。业务错误文本在写入前必须经过对应模块的密钥脱敏。

## 后台与 API

后台菜单“全局异常中心”路由：\`/system-exceptions\`。

- \`GET /api/system/exceptions\`：支持 \`status\`、\`category\`、\`severity\`、\`from\`、\`to\`、\`limit\`；时间参数使用 RFC3339。
- \`GET /api/system/exceptions/count\`：返回待处理/处理中异常数量。
- \`PUT /api/system/exceptions/{id}\`：支持 \`processing\`、\`resolved\`、\`ignored\`；标记解决必须填写处理说明。
- 更新接口在同一数据库事务写入 \`system.exception.update\` 审计日志，记录管理员、来源 IP、User-Agent、目标异常、状态和处理说明。

页面默认筛选待处理记录，可按异常类型、严重程度、状态和日期范围筛选；解决操作必须提交处理结论。

## 运维与排查

1. 先在异常详情查看来源键、关联对象和错误详情。
2. 跳转对应业务对象核实原始记录（订单、采购单、同步任务、登录日志等）。
3. 先完成业务层恢复/人工核实，再在异常中心填写证据和结论并标记解决。
4. 不得仅通过点击“标记解决”来代替业务修复；该操作不会重试发货、补采购、退款或修复 Cap 配置。

迁移：\`0052_system_exceptions.sql\`，应用启动时自动执行，兼容 PostgreSQL 与 SQLite；无新增环境变量或手工 SQL。
