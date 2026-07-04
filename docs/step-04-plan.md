# 第四步计划：自动补池与健康检查

第三步完成 KeyHub 到 `new-api` 的手动上线/停用同步后，第四步进入自动化运营能力。

## 目标

- 按分类保活目标自动从库存补充活跃渠道。
- 周期性检查活跃渠道健康度。
- 对失败渠道自动停用，并记录原因。
- 在控制台展示需要关注的渠道。

## 后端任务

- `pool_refill_worker`：扫描 `category_pool_rules`，按 `keep_alive_target` 计算缺口。
- `health_check_worker`：定时测试 active Key 对应的 new-api 渠道。
- `usage_snapshot_worker`：预留消费快照同步入口。

## 接口

- `PUT /api/pool-rules/{categoryCode}`：设置保活目标和是否启用。
- `POST /api/pool-rules/refill`：手动触发一次补池。
- `POST /api/health-checks/run`：手动触发健康检查。
- `GET /api/health-checks`：查看检查历史。

## 实现策略

- 不引入 Redis，使用 MySQL 行级状态和 Go goroutine worker。
- 单实例 Docker Compose 先用进程内 ticker。
- 后续多实例部署时再加 MySQL advisory lock 或任务表抢占。
- 补池动作复用第三步的 `activate` 同步逻辑。

## 完成标准

- 设置 AWS Bedrock 保活目标为 3 后，点击补池会从库存上线到 3 个 active。
- 补池过程写入 `newapi_sync_events`。
- 健康检查结果写入 `health_checks`。
- 控制台能看到活跃不足、失败、自动禁用的渠道。

## 本步交付

- 后端新增保活规则更新、手动补池、健康检查执行与健康记录查询接口。
- 补池逻辑复用第三步的 `activate` 同步逻辑，将库存 Key 创建为 `new-api` 渠道。
- 健康检查调用 `new-api` 的 `GET /api/channel/test/{id}`，失败时可自动停用本地 Key 和远端渠道。
- 前端控制台支持编辑保活目标、启停自动补池、手动补池、手动健康检查，并展示最近健康检查历史。
