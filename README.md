# KeyHub

KeyHub —— 连接 apikey 提供商与客户平台的桥梁。

KeyHub 是一个给 `new-api` 供给上游供应商 Key 的管理系统。它独立维护 Key 库存、活跃池、健康检查、用量同步和同步记录，并把选中的秘钥发布为接收平台中的 `new-api` 渠道。

## 技术栈

- 后端：Go 标准库 HTTP + MySQL
- 前端：React + TypeScript + Vite + Ant Design
- 部署：Docker Compose
- 任务：Go worker + MySQL 状态表，不依赖 Redis

## 核心能力

- 秘钥管理：导入、去重、加密保存、分类、状态流转和发布渠道 ID 记录。
- 接收平台：维护一个或多个可接收 KeyHub 渠道的 `new-api` 平台。
- 秘钥上线：把本地秘钥转换为 `new-api` channel payload，并推送到选中的接收平台。
- 秘钥下线/禁用：本地禁用秘钥，同时尝试禁用已发布到接收平台的远端渠道。
- 用量同步：从接收平台读取渠道累计用量，并按远端当前全量覆盖本地用量。
- 审计与运维：记录人工操作、同步事件、worker 状态和健康检查结果。
- 用户开放 API：在“API 访问”创建 Token，通过 `/api/v1/key-imports` 批量导入并由后台发布，支持幂等提交、批次查询与安全重试。

## 用户 API 接入

完整说明见 [中文接入指南](docs/public-api.md) 与 [OpenAPI 定义](docs/openapi.json)。部署后可通过页面“API 访问”进入站内文档 `/api-docs.html`；规范地址为 `/openapi.json`（子路径部署需加相应前缀）。

本功能需要执行 `033_add_public_api.sql` 迁移，启用登录鉴权和 worker，并设置加密密钥。API Token 与供应商密钥相互独立，不授予后台管理权限。

## 本地启动

```powershell
Copy-Item .env.example .env
# 修改 .env 里的 KEYHUB_BOOTSTRAP_ADMIN_PASSWORD 和 KEYHUB_ENCRYPTION_KEY
docker compose up --build
```

服务默认监听 `http://localhost:8080`。

首次启动且开启 `KEYHUB_AUTH_ENABLED=true` 时，系统会用
`KEYHUB_BOOTSTRAP_ADMIN_USER` / `KEYHUB_BOOTSTRAP_ADMIN_PASSWORD`
创建第一个管理员。生产环境请设置 `KEYHUB_ENV=production`，并使用真实的
`KEYHUB_ENCRYPTION_KEY`、接收平台凭据和管理员密码。

## 数据库初始化

项目没有单独的 `init.sql`。数据库结构由 `migrations/` 目录维护，首次启动时从 `migrations/001_init.sql` 开始执行，后续版本通过增量迁移演进。

默认 `KEYHUB_AUTO_MIGRATE=true` 时，后端启动会自动执行未完成的迁移。手动部署时请确保：

- `KEYHUB_DATABASE_DSN` 指向目标 MySQL。
- `KEYHUB_MIGRATIONS_DIR` 指向 `migrations` 目录。
- `KEYHUB_ENCRYPTION_KEY` 一旦用于生产数据后保持稳定，否则历史 Token 和秘钥密文将无法解密。

## 接收平台机制

接收平台是 KeyHub 发布秘钥的目标平台。当前支持三种方式：

- `API方式`：接收平台实现 KeyHub 约定接口，KeyHub 使用平台地址和 Token 调用接口。
- `new-api逆向`：接收平台是原生 `new-api`，KeyHub 使用站点地址、账号、密码登录后台，再调用 `new-api` 管理端渠道接口。
- `采购供应商`：接收平台是 `z.tokensupply.cc` 这类供应商采购管理后台，KeyHub 使用账号密码校验后，通过平台的 ApiKey 管理接口录入秘钥。

三种方式都会在 KeyHub 本地保存平台配置。Token、平台登录密码和秘钥正文都会使用 `KEYHUB_ENCRYPTION_KEY` 加密入库。

### API方式

这是标准接入方式，适合接收平台能配合增加 KeyHub 专用接口的场景。

新增接收平台时需要填写：

- 名称：页面展示名。
- 平台地址：`new-api` 或兼容服务的基础地址，不需要带具体接口路径。
- Token：接收平台配置的 `KEYHUB_PUSH_TOKEN`。
- 启用：平台启用后才允许接入；用户是否能向该平台入库由“用户管理”中的接收平台关联决定。

管理员需要在“用户管理”中为每个用户配置零个、一个或多个接收平台。未关联任何已启用平台的用户不能上传秘钥。关联多个平台时，上传页面只向用户展示去重后的渠道类型，不展示平台名称；用户可以选择一种、多种或全选，自动入库时会推送到所有匹配所选渠道类型的已启用关联平台。

用户选择渠道类型后，上传页面会取任一个匹配接收平台的模型列表供用户编辑。未编辑时，各平台仍使用各自配置的模型；编辑后，本次创建的所有匹配上游渠道使用用户编辑后的模型列表。

接收平台必须实现：

```http
POST /api/keyhub/channels
Authorization: Bearer <KEYHUB_PUSH_TOKEN>
Content-Type: application/json
```

KeyHub 上线秘钥时会发送 `source`、`external_id`、`operation` 和 `channel`。接收平台创建或更新渠道后，需要返回远端 `channel_id`，KeyHub 会把它保存为发布渠道 ID。

接收平台还必须实现：

```http
GET /api/keyhub/channels/usage
Authorization: Bearer <KEYHUB_PUSH_TOKEN>
Accept: application/json
```

返回数据中的 `channel_id` 必须与上线接口返回的 ID 一致，`used_quota` 是该渠道累计用量。KeyHub 点击“同步”或 worker 自动同步时，会把远端当前累计值作为全量用量写入本地；同一天重复同步会覆盖该渠道当天记录，不再按增量追加。

新增或编辑 API 方式平台时，KeyHub 会调用用量接口做握手校验。接口缺失、鉴权失败、响应格式错误或返回负数用量时，平台无法保存。

### new-api逆向

这种方式适合无法修改接收端 `new-api` 代码，但拥有后台账号的场景。

新增接收平台时需要填写：

- 名称、平台地址。
- 账号、密码。
- 启用：只有启用的平台才允许接入。

账号需要具备号池管理员权限。点击“检查管理员权限”时，KeyHub 会执行：

1. `POST /api/user/login` 使用账号密码登录。
2. 读取登录响应中的用户 ID，并保存服务端返回的 Cookie。
3. 携带 Cookie 和 `New-Api-User: <用户ID>` 请求 `GET /api/channel/`。
4. 能正常查询渠道列表才视为校验通过。

当前不支持需要二次验证或邮箱验证码的账号。逆向方式只保存配置和执行必要的后台调用，不会模拟浏览器页面操作。

### 采购供应商

这种方式适合 `https://z.tokensupply.cc` 的采购供应商管理平台。该平台新增 ApiKey 时，前端实际调用：

```http
POST /api/apikey/add
x-provider-user: <username>
Content-Type: application/json
```

请求体字段包括 `api_key`、`provider_slug`、`has_quota_limit`、`quota_limit`、`description` 和 `biz_date`。KeyHub 会把本地秘钥映射为 `api_key`，按渠道类型映射 `provider_slug`，并使用当天 `yyyyMMdd` 作为 `biz_date`。

已确认的供应商映射：

- OpenAI -> `openai`
- Anthropic -> `anthropic`
- Azure OpenAI -> `azure`
- 旧版 AWS Bedrock（new-api `type=33`）-> `amazon-bedrock`
- 新版 AWS Bedrock（复用 Anthropic `type=14`）-> `anthropic`
- Google AI Studio -> `google-vertex`

平台登录接口是 `POST /api/user/login`，后续列表、添加、删除请求使用 `x-provider-user` 标识账号。该平台返回的 ApiKey ID 是字符串，KeyHub 会将其稳定映射成本地数字发布 ID；下线和用量同步时再通过远端列表反查原始字符串 ID。

消费同步会读取 `GET /api/apikey/list` 返回的 `actual_cost`，按 `KEYHUB_NEWAPI_QUOTA_PER_USD` 转换为 KeyHub 的累计 quota 后按全量写入本地用量。

### 上线、同步、下线闭环

秘钥上线流程：

1. 在“我的秘钥”中选择秘钥并点击“上线”。
2. 选择一个已启用的接收平台。
3. KeyHub 解密本地秘钥，转换为 `new-api` channel 参数。
4. `API方式` 调用 `POST /api/keyhub/channels`；`new-api逆向` 登录后调用 `POST /api/channel/`；`采购供应商` 调用 `POST /api/apikey/add`。
5. 成功后 KeyHub 写入发布绑定关系，记录接收平台内部 ID、远端渠道 ID 和同步事件。

AWS Bedrock 分为两种渠道类型：旧版保持 new-api `type=33`，并要求选择密钥类型；AK/SK 使用 `AccessKey|SecretAccessKey|Region`，API Key 使用 `APIKey|Region`，该选择会写入 `settings.aws_key_type`。新版复用 Anthropic 的 new-api `type=14` 和 payload 结构，不写入 AWS 设置，但渠道名称使用 `${用户名}_aws_bedrock_new` 与 Anthropic 渠道区分。上传新版 AWS Bedrock 秘钥时必须提供至少一个以 `https://bedrock-mantle` 开头的区域地址，系统按“区域 × 秘钥”展开记录，并在通过 `new-api 逆向`创建渠道时将区域地址写入 `base_url`。新版渠道还会根据最终模型列表自动生成 `model_mapping`，把每个模型映射到 `anthropic.${模型ID}`。

消费同步流程：

1. 点击“我的秘钥”的“同步用量”，或开启 `KEYHUB_WORKER_ENABLED=true` 由 worker 定时执行。
2. KeyHub 只同步本地状态为 active、发布绑定也为 active 的秘钥。
3. `API方式` 调用 `/api/keyhub/channels/usage`。
4. `new-api逆向` 登录后分页读取 `/api/channel/` 的 `used_quota`；`采购供应商` 读取 `/api/apikey/list` 的 `actual_cost`。
5. KeyHub 按接收平台和渠道 ID 匹配绑定关系，将远端当前全量用量覆盖写入 `usage_daily_snapshots`。

下线/禁用流程：

1. 在“我的秘钥”中禁用秘钥。
2. KeyHub 查找该秘钥所有发布绑定。
3. `API方式` 再次调用 `/api/keyhub/channels`，用禁用状态更新远端渠道。
4. `new-api逆向` 登录后调用 `PUT /api/channel/`，把远端渠道设置为手动禁用。
5. 远端禁用成功后，本地绑定标记为 disabled，本地秘钥标记为 disabled；失败时会记录同步错误，便于重试或排查。

## 环境变量

常用配置见 `.env.example`：

- `KEYHUB_HTTP_ADDR`：后端监听地址，默认 `:8080`。
- `KEYHUB_DATABASE_DSN`：MySQL DSN。
- `KEYHUB_AUTO_MIGRATE`：是否启动时自动执行迁移。
- `KEYHUB_STATIC_DIR`：生产模式下 React 构建产物目录。
- `KEYHUB_ENCRYPTION_KEY`：加密平台 Token、逆向密码和秘钥正文的密钥，生产环境必须替换。
- `KEYHUB_AUTH_ENABLED`：是否启用登录鉴权，生产环境必须开启。
- `KEYHUB_BOOTSTRAP_ADMIN_USER` / `KEYHUB_BOOTSTRAP_ADMIN_PASSWORD`：初始化管理员。
- `KEYHUB_WORKER_ENABLED`：是否开启后台 worker，默认开启。
- `KEYHUB_USAGE_SYNC_INTERVAL`：消费同步 worker 间隔，默认 `1m`。
- `KEYHUB_AGGREGATION_TARGETS_JSON`：可选的初始接收平台配置；每个平台应配置 `channelType` 和 `models`，启动后也可以在页面维护。

## 开发命令

```powershell
go test ./...
cd web
npm install
npm run build
```

## 运维命令

备份当前 Docker Compose MySQL 数据库：

```powershell
.\scripts\backup-mysql.ps1
```

从备份恢复到当前 `keyhub` 数据库：

```powershell
.\scripts\restore-mysql.ps1 -BackupFile .\backups\keyhub-20260625-120000.sql -ConfirmRestore
```

建议升级流程：

1. 执行 `.\scripts\backup-mysql.ps1`。
2. 拉取或构建新版本镜像。
3. 使用 `docker compose up -d --build` 启动，后端会在 `KEYHUB_AUTO_MIGRATE=true` 时执行迁移。
4. 打开“运维状态”页面确认 MySQL、new-api 管理接口、静态资源和 worker 状态。

如需 Docker 日志轮转，可在 `docker-compose.yml` 的服务下加入：

```yaml
logging:
  driver: json-file
  options:
    max-size: "20m"
    max-file: "5"
```

## 开源注意事项

开源前请确认不要提交生产配置和敏感文件：

- 不要提交 `.env`、生产 `docker-compose-prod.yml`、数据库备份、日志、证书私钥或真实 Token。
- 不要在 README、测试或前端文档中保留真实公网 IP、账号、密码、Token、Cookie、加密密钥。
- 如果生产密钥曾经被提交、复制到远程构建上下文或共享给第三方，应立即轮换 `KEYHUB_ENCRYPTION_KEY` 以外的可轮换凭据；`KEYHUB_ENCRYPTION_KEY` 轮换前需要先规划历史密文迁移。
- 生产环境建议启用 HTTPS，并设置 `KEYHUB_COOKIE_SECURE=true`。
