# KeyHub 用户 API 接入指南

版本：v1。通过脚本上传供应商密钥，由 KeyHub 发布到当前用户关联的接收平台。

## 1. 开始使用

登录 KeyHub，打开侧栏“API 访问”，创建并保存 API Token。完整 Token 只显示一次，遗失后请撤销并重新创建。

- KeyHub API Token 用于访问本系统；请求体中的 keys 是要上传的供应商密钥，二者不可混用。
- 管理员须先为用户关联已启用的接收平台。账号管理员创建的下属账号继承其平台关联。
- Token 只能导入密钥、读取渠道选项、查询和重试本人批次。管理员的 Token 也不能调用管理接口或查看其他用户批次。
- Token 有效期为 1–365 天，默认 90 天；每个用户最多 10 个有效 Token。页面支持撤销及查看最近使用时间。
- 用户禁用后 Token 立即不可用；撤销 Token 不取消已经受理的任务。任务执行前仍检查用户状态和平台权限。

API 基础地址：`https://你的域名/api/v1`。部署在子路径时，在域名后保留该子路径，以“API 访问”页面展示的地址为准。生产环境通过 HTTPS 调用。

所有请求携带 `Authorization: Bearer kh_...`。JSON 请求使用 `Content-Type: application/json`。Cookie 不能作为 v1 API 的凭据。

可下载 [OpenAPI 定义](openapi.json) 导入 Postman 等工具。

## 2. 查询可选渠道

`GET /api/v1/key-options`

```bash
export KEYHUB_API_TOKEN='替换为你的 KeyHub API Token'
export KEYHUB_API_BASE='https://你的域名/api/v1'
curl "$KEYHUB_API_BASE/key-options" \
  -H "Authorization: Bearer $KEYHUB_API_TOKEN"
```

```json
{
  "items": [
    {"channelType":"anthropic","models":["your-model-id"],"targetCount":2}
  ],
  "limits": {
    "maxKeys":100,"maxExpandedKeys":100,"maxPublishTasks":500,
    "readRequestsPerMinute":120,"writeRequestsPerMinute":10
  },
  "publishingEnabled":true
}
```

仅返回当前用户获准使用的渠道类型，不公开接收平台名称、地址或凭据。`models` 是同类型任一平台的示例模型列表，不代表所有平台模型的并集；`targetCount` 是该类型关联的平台数。

## 3. 提交密钥

`POST /api/v1/key-imports`

必须携带 `Idempotency-Key`，由客户端为每个新批次生成并保存。建议使用 UUID。格式为 8–128 位 ASCII 字母、数字或 `_ . : -`，区分大小写。

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| keys | 是 | 1–100 个非空单行字符串；每项最多 8192 字节，自动去除首尾空白 |
| channelTypes | 是 | 1–6 个不重复的渠道类型；必须全部属于当前用户可用类型 |
| models | 否 | 本次所有发布渠道的模型覆盖值；最多 200 项，每项最多 128 字节 |
| awsCredentialType | 条件必填 | 旧版 AWS 专用，值为 ak_sk 或 api_key |
| awsBedrockRegions | 条件必填 | 新版 AWS 专用，1–20 个区域地址，每项最多 512 字节 |

不传 `models` 或传空数组时，各接收平台使用各自的模型配置；传非空数组时覆盖本次全部发布渠道。用户不能在请求中指定密钥所有者或接收平台 ID。

```bash
curl -X POST "$KEYHUB_API_BASE/key-imports" \
  -H "Authorization: Bearer $KEYHUB_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: example-batch-0001' \
  --data '{"keys":["SUPPLIER_KEY_1","SUPPLIER_KEY_2"],"channelTypes":["anthropic"]}'
```

首次受理返回 `202 Accepted`，表示本地密钥和后台发布任务已持久化，不表示远端发布已完成。同一用户使用相同幂等标识和相同请求重试，返回 `200 OK` 及原批次的当前状态；更换 Token 后仍可重放。

```json
{
  "id":"0123456789abcdef0123456789abcdef",
  "status":"queued",
  "createdAt":"2026-09-18T08:00:00Z",
  "total":2,"imported":2,"duplicates":0,
  "publishTotal":2,"succeeded":0,"failed":0,"needsReview":0,"pending":2,
  "items":[
    {"inputIndex":0,"keyId":101,"importStatus":"imported","deliveries":[{"channelType":"anthropic","status":"pending"}]},
    {"inputIndex":1,"keyId":102,"importStatus":"imported","deliveries":[{"channelType":"anthropic","status":"pending"}]}
  ]
}
```

`inputIndex` 从 0 开始，对应原始 keys 数组。`keyId` 是 KeyHub 本地密钥 ID。`total` 是区域展开后的记录数；发布计数按“记录 × 平台”计算。相同渠道类型可以有多个发布结果，不暴露平台身份。

重复密钥返回 `importStatus: "duplicate"`，不返回既有密钥的 ID，也不创建发布任务；全局去重不改变原有密钥的所有者。需要重试已创建批次时，请使用批次重试接口。

## 4. 六类渠道参数

| channelTypes 值 | keys 中每一项的格式 | 额外参数 |
| --- | --- | --- |
| anthropic | 供应商密钥原文 | 无 |
| openai | 供应商密钥原文 | 无 |
| azure_openai | 接收平台支持的凭据原文 | 无 |
| google_ai_studio | 接收平台支持的凭据原文 | 无 |
| aws_bedrock | AccessKey、SecretAccessKey、Region 用竖线连接，或 APIKey、Region 用竖线连接 | awsCredentialType 必填 |
| aws_bedrock_new | 供应商密钥原文 | awsBedrockRegions 必填，且该类型须单独提交 |

前四类保持密钥原文，具体有效性由接收平台判定。可把普通示例中的 `channelTypes` 替换为对应值。

旧版 AWS AK/SK 示例：

```json
{"keys":["ACCESS_KEY|SECRET_KEY|us-east-1"],"channelTypes":["aws_bedrock"],"awsCredentialType":"ak_sk"}
```

旧版 AWS API Key 示例：

```json
{"keys":["BEDROCK_API_KEY|us-east-1"],"channelTypes":["aws_bedrock"],"awsCredentialType":"api_key"}
```

新版 AWS 示例：

```json
{
  "keys":["BEDROCK_KEY_1","BEDROCK_KEY_2"],
  "channelTypes":["aws_bedrock_new"],
  "awsBedrockRegions":[
    "https://bedrock-mantle.us-east-1.api.aws",
    "https://bedrock-mantle.us-west-2.api.aws"
  ]
}
```

新版按“区域 × 密钥”展开；此例产生 4 条记录。区域地址必须以 `https://bedrock-mantle` 开头。逆向接收平台将其写入渠道 base_url；最终模型会映射为 `anthropic.${模型ID}`。每条结果的 `baseUrl` 标识对应输入区域。

## 5. 查询结果与重试

`GET /api/v1/key-imports/{id}` 返回与提交接口相同的批次结构。建议每 3–5 秒查询一次，直到 `pending` 为 0。

```bash
curl "$KEYHUB_API_BASE/key-imports/0123456789abcdef0123456789abcdef" \
  -H "Authorization: Bearer $KEYHUB_API_TOKEN"
```

| 批次 status | 含义 |
| --- | --- |
| queued | 发布或核查任务等待执行 |
| running | 有任务正在执行 |
| succeeded | 全部发布成功，或本批全部为跳过的重复密钥 |
| partial_failed | 部分发布成功，部分可重试失败 |
| failed | 发布均失败，检查逐项错误 |
| needs_review | 至少一项远端结果无法确认，需要核查 |

每个 delivery 的状态为 `pending`、`running`、`succeeded`、`failed`、`needs_review` 或 `reconcile_pending`。查询结果是截至查询时的进度；批次待处理数量归零后再判断最终结果。

`POST /api/v1/key-imports/{id}/retry` 不需要请求体或 Idempotency-Key，返回 `202`。重复调用不会再次发布已成功或正在处理的任务。

```bash
curl -X POST "$KEYHUB_API_BASE/key-imports/0123456789abcdef0123456789abcdef/retry" \
  -H "Authorization: Bearer $KEYHUB_API_TOKEN"
```

- `failed`：修复权限或配置后重新尝试；执行前重新校验权限。
- `needs_review`：只查询远端已有结果，不再次创建渠道。new-api 逆向和采购供应商通过唯一批次标识核查；标准 API 接入缺少按外部 ID 查询的接口，无法确认时需联系管理员核实。
- 已受理任务不会因 HTTP 客户端断开而丢失。服务重启后继续未开始的任务，已开始但未完成的任务先核查。
- 远端网络调用不具备跨系统事务能力，API 不承诺远端 exactly-once。不要通过新幂等标识绕过待核查状态。

## 6. Python 示例

仅使用 Python 标准库。事先在环境变量中配置 KEYHUB_API_BASE 和 KEYHUB_API_TOKEN，把真实上传数据写入本地 payload.json；不要提交该文件。

```python
import json
import os
import time
import urllib.error
import urllib.request

base = os.environ["KEYHUB_API_BASE"].rstrip("/")
token = os.environ["KEYHUB_API_TOKEN"]
# 为该业务批次生成一次并持久保存；超时重试时必须沿用。
request_id = "my-supplier-batch-20260918-001"
with open("payload.json", encoding="utf-8") as f:
    payload = json.load(f)

def call(method, path, body=None, idempotency=None):
    headers = {"Authorization": "Bearer " + token}
    if idempotency:
        headers["Idempotency-Key"] = idempotency
    data = None
    if body is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    request = urllib.request.Request(base + path, data=data,
                                     headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        # 429 时根据 Retry-After 等待。提交超时或 5xx 时沿用原 request_id。
        raise RuntimeError(error.read().decode()) from error

job = call("POST", "/key-imports", payload, request_id)
print("批次：", job["id"])
while job["pending"]:
    time.sleep(5)
    job = call("GET", "/key-imports/" + job["id"])
print(json.dumps(job, ensure_ascii=False, indent=2))
```

## 7. 错误与限额

HTTP 错误统一返回：

```json
{"error":{"code":"CHANNEL_NOT_ALLOWED","message":"用户未关联所选渠道类型的已启用平台"}}
```

| HTTP | code | 处理方式 |
| --- | --- | --- |
| 400 | INVALID_REQUEST / INVALID_IDEMPOTENCY_KEY / BATCH_TOO_LARGE | 修正字段、幂等标识或缩小批次 |
| 401 | INVALID_TOKEN | 检查 Token、有效期、撤销状态和用户状态 |
| 403 | CHANNEL_NOT_ALLOWED | 联系管理员配置平台关联 |
| 404 | NOT_FOUND | 批次不存在或不属于当前用户 |
| 405 | METHOD_NOT_ALLOWED | 使用接口规定的方法 |
| 409 | IDEMPOTENCY_CONFLICT | 新批次用新标识；旧批次重试保持原请求 |
| 409 | REQUEST_IN_PROGRESS | 按 Retry-After 等待后重试原请求 |
| 429 | RATE_LIMITED | 按 Retry-After 等待后重试 |
| 500 | INTERNAL_ERROR | 查询原批次，或使用相同幂等标识重试提交 |
| 503 | AUTH_REQUIRED / PUBLISHING_DISABLED / ENCRYPTION_UNAVAILABLE | 联系管理员检查服务配置 |

逐项发布错误：`OWNER_UNAVAILABLE`、`CHANNEL_NOT_ALLOWED`、`KEY_UNAVAILABLE`、`TARGET_UNAVAILABLE`、`TARGET_CHANGED`、`TARGET_CONFIGURATION_INVALID`、`REMOTE_OUTCOME_UNKNOWN`。前几项表示权限、记录或配置需要修复；`REMOTE_OUTCOME_UNKNOWN` 需核查远端结果。更改接收平台地址、接入方式、渠道类型或逆向账号后，已有任务不会自动转投新目的地。

- 每用户每自然分钟最多 120 次读取、10 次写入，所有 Token 和服务实例共享计数。请求重放和重试也计入限额。
- 每次最多 100 个输入密钥、20 个区域地址、100 条展开记录、500 个发布任务，请求体不超过 2 MiB。
- 幂等标识在同一用户内唯一，随批次保存；当前不自动清理。数组顺序属于请求内容，重试保持顺序不变。
- v1 不提供删除密钥、读取密钥明文或更改接收平台的能力。

## 8. 部署与运维

部署本版本后执行迁移 `033_add_public_api.sql`；默认启动自动迁移。无需 Redis，任务和限流存储在 MySQL。

需启用 `KEYHUB_AUTH_ENABLED=true`、`KEYHUB_WORKER_ENABLED=true`，并设置稳定的 `KEYHUB_ENCRYPTION_KEY`。后台发布按数据库串行处理，各实例通过 MySQL 连接锁协调。停用 worker 时仍可查询历史批次和重放请求，但不接受新批次或重试。

运维与审计记录包含 `api_publish`、`api.keys.import`、`api.keys.publish`、`api.keys.retry`、`api_token.create`、`api_token.revoke`。Token 最近使用时间按分钟更新。批次接口不返回供应商密钥明文或接收平台内部信息。

## 9. 文档与测试维护

中文文档源文件为 docs/public-api.md；接口规范源文件为 docs/openapi.json。执行 `node scripts/build-api-docs.mjs` 生成站内静态文档，前端开发与构建时也会自动生成。

后端常规检查：`go test ./...`。集成测试通过 `KEYHUB_TEST_MYSQL_DSN` 显式指定隔离的测试 MySQL；测试会创建并删除随机命名的 keyhub_api_test_* 数据库，需要 CREATE DATABASE 权限。不要指向生产 MySQL。
