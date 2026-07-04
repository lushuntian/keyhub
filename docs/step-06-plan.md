# 第六步计划：权限、安全与生产部署收口

第五步已经完成消费快照和后台调度。第六步进入生产可用性收口。

## 目标

- 增加 KeyHub 自身登录鉴权，避免管理接口裸露。
- 为危险操作增加确认、审计和最小权限控制。
- 生产部署改为 `.env` 注入密钥，不再依赖示例配置。
- 补充基础运维文档：备份、升级、回滚、端口冲突处理。

## 后端任务

- 增加本地管理员账号表和 session cookie 鉴权。
- 给 `/api/*` 管理接口加认证中间件，保留 `/api/health` 可公开探活。
- 加强加密密钥启动校验，禁止生产使用示例密钥。
- 增加审计查询接口，用于查看人工和 worker 操作。

## 前端任务

- 增加登录页和会话过期处理。
- 在停用、自动停用、批量同步等操作前加入确认弹窗。
- 增加审计日志页面。

## 完成标准

- 未登录无法访问控制台数据和管理接口。
- Docker Compose 使用 `.env` 管理所有敏感配置。
- `go test ./...`、`npm run build`、`docker compose config` 全部通过。

## 本步交付

- 新增 `admin_users` 和 `admin_sessions`，使用 HttpOnly session cookie 做本地管理员鉴权。
- 新增登录、退出、当前会话接口：
  - `POST /api/auth/login`
  - `POST /api/auth/logout`
  - `GET /api/auth/me`
- 除 `/api/health` 和登录相关接口外，所有 `/api/*` 管理接口已加鉴权保护。
- 新增 `GET /api/audit/logs` 和前端“审计日志”页面。
- 前端增加登录页、退出按钮、会话过期处理，以及停用渠道/健康检查自动停用前的确认弹窗。
- Docker Compose 不再直接加载 `.env.example`，改为从 `.env`/环境变量注入敏感配置。
- 生产模式 `KEYHUB_ENV=production` 会拒绝示例加密密钥、关闭鉴权、空 new-api token 等不安全配置。
