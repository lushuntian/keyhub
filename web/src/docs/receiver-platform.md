# apikey 接收方式

## 接收方式

KeyHub 在“我的秘钥”页面点击“上线”后，会把选中的秘钥渠道推送到接收平台。接收平台必须实现 KeyHub 接入接口；如果缺少任一必选接口，KeyHub 会拒绝保存或拒绝上线该平台。

## 必选接口

### 接收或更新渠道

```http
POST /api/keyhub/channels
Authorization: Bearer <KEYHUB_PUSH_TOKEN>
Content-Type: application/json
```

请求体：

```json
{
  "source": "keyhub",
  "external_id": "api_key:21",
  "operation": "upsert",
  "channel": {
    "type": 18,
    "key": "ak|sk|region",
    "name": "keyhub-aws_bedrock-21",
    "models": "claude-opus-4-6,claude-opus-4-7",
    "status": 2,
    "tag": "aws_bedrock"
  }
}
```

响应体：

```json
{
  "success": true,
  "message": "",
  "data": {
    "channel_id": 123,
    "action": "created",
    "external_id": "api_key:21"
  }
}
```

### 查询渠道累计额度

基于诚信原则，接收平台必须提供 KeyHub 专用用量接口。KeyHub 不再依赖 new-api 管理端 `/api/channel/` 列表推断消费；所有接收平台都应明确返回自己保存的渠道累计额度。

```http
GET /api/keyhub/channels/usage
Authorization: Bearer <KEYHUB_PUSH_TOKEN>
Accept: application/json
```

响应体：

```json
{
  "success": true,
  "message": "",
  "data": {
    "items": [
      {
        "channel_id": 123,
        "used_quota": 456789,
        "request_count": 42
      }
    ],
    "total": 1
  }
}
```

字段说明：

- `channel_id`：接收平台返回给 KeyHub 的渠道 ID，必须与上线接口返回的 `channel_id` 一致。
- `used_quota`：渠道累计消耗额度，必须是单调递增的非负整数；如果平台发生清零，KeyHub 会按重置后的当前值重新计算增量。
- `request_count`：可选字段，表示累计请求数；当前 KeyHub 只用 `used_quota` 计算消费。

KeyHub 在新增或编辑接收平台时会调用该接口做握手校验；接口缺失、鉴权失败、返回格式不合法或返回负数额度，都会导致平台无法接入。

接收平台地址只需要填写 new-api 的基础地址。KeyHub 会自动拼接接口路径并携带 Token。

## new-api 配置方法

- 在 new-api 启动环境中配置 `KEYHUB_PUSH_TOKEN`。
- 重启 new-api，使 Token 生效。
- new-api 必须同时实现 `POST /api/keyhub/channels` 和 `GET /api/keyhub/channels/usage`。
- new-api 接收到渠道后会按 KeyHub 绑定关系创建或更新 channel。
- 上线后的 channel 默认禁用，默认分组为 `default`，需要在 new-api 中确认后再启用。

## KeyHub 配置方法

- 打开“接收平台”页面，点击“新增平台”。
- 标识建议使用 `new-api`，名称填写便于识别的环境名。
- 平台地址填写 new-api 基础地址，不需要填写 `/api/keyhub/channels`。
- Token 填写与 new-api `KEYHUB_PUSH_TOKEN` 一致的值。
- 勾选“启用”后，该平台才会出现在上线弹窗中；勾选“默认”后会作为默认选择。
- KeyHub 后端需要保持稳定的 `KEYHUB_ENCRYPTION_KEY`，用于加密保存平台 Token 和秘钥正文。

## 上线流程

1. 在 KeyHub 导入或维护秘钥库存。
2. 在“我的秘钥”中点击“管理”，选择“上线”。
3. 选择一个已启用的接收平台并确认。
4. KeyHub 解密本地秘钥，转换成 new-api channel 参数并推送。
5. new-api 返回 channel 结果后，KeyHub 记录上线绑定关系。

## 秘钥形态兼容

- OpenAI、Anthropic、Gemini 等单 Key 渠道会按单个秘钥推送。
- AWS Bedrock 支持 `AccessKey|SecretKey|Region` 或 `ApiKey|Region` 形态。
- Azure OpenAI 会拆分 `Endpoint|ApiKey|ApiVersion`，并写入 channel 的地址、秘钥和额外参数。
- 模型、端点、预估 TPM、备注等字段会尽量映射到 new-api channels 表的对应参数。

## 常见问题

- 如果提示 `401` 或鉴权失败，请确认 KeyHub 平台 Token 与 new-api `KEYHUB_PUSH_TOKEN` 完全一致。
- 如果新增平台提示必须实现 `GET /api/keyhub/channels/usage`，说明接收平台版本过旧或返回格式不符合 KeyHub 契约，需要先升级接收平台。
- 如果提示 `decrypt ciphertext`，通常是 `KEYHUB_ENCRYPTION_KEY` 变更导致旧数据无法解密，需要恢复原来的密钥或重新导入数据。
- 如果上线后 new-api 看不到可用渠道，请确认 channel 默认是禁用状态，需要在 new-api 中人工启用。
