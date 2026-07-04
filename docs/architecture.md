# KeyHub 架构设计

## 系统定位

KeyHub 是供应商侧 Key 池管理系统，不直接参与用户请求转发。它负责把供应商提供的上游 Key 维护成可用库存，并同步成 `new-api` 渠道。

## 第一版边界

- KeyHub 自己管理 Key 生命周期：库存、活跃、禁用、异常、吊销。
- `new-api` 作为同步目标：KeyHub 生成渠道 payload，并记录 `newapi_channel_id`。
- 生产环境前后端不分离：Go 进程托管 React 构建目录。
- 不使用 Redis：健康检查、消费同步等后台任务后续通过 MySQL 表锁/状态字段协调。

## 与 new-api 对齐的字段

参考 `D:\new\new-api`：

- `model.Channel.Type`：渠道类型。当前设计用到 OpenAI=1、Azure=3、Anthropic=14、Gemini=24、AWS=33。
- `model.Channel.Key`：上游 key。AWS Bedrock 支持 `AK|SK|Region`，new-api 的 AWS adapter 也支持 `ApiKey|Region`。
- `model.Channel.Models`：逗号分隔模型列表。
- `model.Channel.Group`：用户分组，new-api 内部按逗号包裹匹配。
- `model.Channel.Tag`：渠道标签，用于批次/分组管理。
- `model.Channel.OtherSettings`：AWS/Azure 等渠道特殊设置。
- `model.Channel.AutoBan/Priority/Weight`：自动禁用和调度权重。

## 模块规划

- API Key 管理：导入、去重、加密、脱敏、批次记录。
- 渠道同步：把 KeyHub 的 Key 生成 new-api channel，支持创建、更新、禁用、删除。
- 健康检查：调用 new-api 或上游轻量探测，记录成功率和错误。
- 消费快照：从接收平台的 KeyHub 专用用量接口拉取渠道累计额度并按天沉淀。
- 审计：记录人工操作和自动任务行为。

## 第一版数据库

- `category_pool_rules`：分类配置。
- `key_import_batches`：导入批次。
- `api_keys`：Key 主表，保存生命周期状态和 new-api 同步状态。
- `newapi_sync_events`：同步事件。
- `health_checks`：健康检查历史。
- `usage_daily_snapshots`：每日消耗快照。
- `audit_logs`：审计日志。
