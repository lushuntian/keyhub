# 第五步计划：消费快照与后台调度

第四步已经具备人工触发的自动补池和健康检查。第五步把系统从“可操作”推进到“可持续运行”。

## 目标

- 从 `new-api` 拉取渠道消费数据，按天写入 `usage_daily_snapshots`。
- 在 Go 进程内启动轻量 worker，定时执行补池、健康检查和消费快照。
- 不引入 Redis，使用 MySQL 状态字段和后续可扩展的 advisory lock 做任务协调。
- 前端“消费快照”页面展示分类、标签、渠道维度的用量趋势。

## 后端任务

- 扩展 `internal/newapi/client.go`，对接 `new-api` 渠道日志或统计接口。
- 新增 `usage_repository`，负责按日期和渠道 upsert 快照。
- 新增 worker runner，支持环境变量配置开关和执行间隔。
- 记录 worker 执行审计，失败时写入 `audit_logs`，不影响主 HTTP 服务。

## 前端任务

- 实现消费快照页：总消费、近 7 天趋势、分类排行、渠道明细。
- 为自动任务增加只读状态区：上次执行时间、成功/失败、最近错误。

## 完成标准

- `POST /api/usage/sync` 可以手动同步一次消费数据。
- worker 启用后能按配置间隔自动补池、健康检查、同步消费。
- `docker compose config`、`go test ./...`、`npm run build` 全部通过。

## 本步交付

- 新增 `usage_sync_cursors`，用接收平台 KeyHub 专用接口返回的渠道累计 `used_quota` 计算每日增量，首次同步只建立基线。
- 新增 `worker_runs`，记录补池、健康检查、消费同步 worker 的执行状态。
- 新增接口：
  - `POST /api/usage/sync`
  - `GET /api/usage/summary`
  - `GET /api/workers/runs`
- Go 进程内 worker 已接入，默认关闭，通过 `KEYHUB_WORKER_ENABLED=true` 启用。
- 前端“消费快照”页已实现总览、日趋势、分类排行、渠道明细和 worker 状态。
