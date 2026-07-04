# 第七步计划：备份、恢复与可观测性

第六步已经完成权限和生产部署收口。第七步进入运维可靠性建设。

## 目标

- 提供 MySQL 备份与恢复脚本。
- 增加运行状态和任务失败的可观测性。
- 给关键表提供导出能力，便于迁移或审计。

## 后端任务

- 新增只读运维接口：数据库表统计、最近错误、worker 成功率。
- 增加 Key 导出清单接口，只导出脱敏信息，不导出明文 Key。
- 增加健康检查细分：数据库、new-api 管理 API、静态资源状态。

## 部署任务

- 增加 `scripts/backup-mysql.ps1` 和 `scripts/restore-mysql.ps1`。
- 文档补充升级流程：备份、拉取镜像、执行迁移、回滚。
- 给 Docker Compose 增加可选日志轮转配置说明。

## 完成标准

- 可以一条命令备份 KeyHub 数据库。
- 可以从备份恢复到新 MySQL 卷。
- 运维页面可以看到最近 worker 失败、new-api 连接异常和数据库状态。

## 本步交付

- 新增 `/api/ops/status`，返回系统健康、MySQL 表容量、worker 近 7 天成功率和最近错误。
- 扩展 `/api/health`，细分数据库、new-api 管理接口和前端静态资源状态。
- 新增 `/api/keys/export`，只导出 Key 脱敏信息、分类、状态、new-api 渠道号和错误计数。
- 前端新增“运维状态”页面，支持查看组件状态、worker 成功率、最近错误，并下载脱敏 Key 清单。
- 新增 `scripts/backup-mysql.ps1` 和 `scripts/restore-mysql.ps1`，README 补充备份、恢复、升级和日志轮转说明。

## 下一步规划

Step 08 建议进入“权限细化与生产体验”：

- 增加管理员列表、修改密码、会话失效和角色字段的页面能力。
- 区分 viewer/operator/admin 权限，导出、停用、补池等高风险动作仅允许 operator/admin。
- 为后台 worker 增加 MySQL 分布式锁，支持多实例部署时避免重复补池、重复健康检查和重复用量同步。
- 增加运行配置页面，至少展示只读配置、worker 开关状态和最近启动信息。
