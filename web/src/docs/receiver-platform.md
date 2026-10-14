# apikey 接收方式

## 接收方式

KeyHub 在“我的秘钥”页面点击“上线”后，会把选中的秘钥渠道推送到接收平台。接收平台必须实现 KeyHub 接入接口；如果缺少任一必选接口，KeyHub 会拒绝保存或拒绝上线该平台。

除标准接口外，KeyHub 还内置了两类兼容模式：

- `new-api逆向`：使用 new-api 管理员账号登录后台并调用 `/api/channel/`。
- `采购供应商`：使用 `z.tokensupply.cc` 这类供应商后台的 `/api/apikey/*` 接口录入和管理 ApiKey。

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
    "type": 33,
    "key": "ak|sk|region",
    "name": "supplier_aws_bedrock",
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
- 上线后的 channel 默认启用，默认分组为 `default`。

## KeyHub 配置方法

- 打开“接收平台”页面，点击“新增平台”。
- 名称填写便于识别的环境名；平台 ID 由系统自动生成。
- 渠道类型选择该平台要接收的供应商类型，例如 Anthropic、旧版 AWS Bedrock、新版 AWS Bedrock、OpenAI、Azure OpenAI 或 Google AI Studio。
- 模型列表由管理员在接收平台中手动维护，可输入多个模型；同步时严格以该平台的模型列表为准。
- 平台地址填写 new-api 基础地址，不需要填写 `/api/keyhub/channels`。
- Token 填写与 new-api `KEYHUB_PUSH_TOKEN` 一致的值。
- 勾选“启用”后，该平台才允许接入；用户是否能向该平台入库由“用户管理”中的接收平台关联决定。
- “系统默认接收平台”同一时间最多只能配置一个，且必须使用 `new-api逆向` 接入方式；“使用日志”会实时读取该平台，但该配置仍不参与秘钥分配、上线或路由逻辑。
- “使用日志”对所有登录用户开放：普通用户仅可查询本人秘钥对应的上游渠道，账号管理员可查询自己及直属下属用户秘钥对应的上游渠道，系统管理员可查询全部已映射渠道。
- 管理员可为每个用户关联零个、一个或多个接收平台；未关联任何已启用平台的用户不能上传秘钥。
- 用户关联多个平台时，上传页只展示去重后的渠道类型，不展示平台名称；用户可选择一种、多种或全选。
- 密钥会自动入库到所有匹配用户所选渠道类型的已启用关联平台。
- KeyHub 后端需要保持稳定的 `KEYHUB_ENCRYPTION_KEY`，用于加密保存平台 Token 和秘钥正文。

## 采购供应商模式

`z.tokensupply.cc` 的 ApiKey 录入接口如下：

```http
POST /api/apikey/add
x-provider-user: <username>
Content-Type: application/json
```

KeyHub 发送的主要字段：

```json
{
  "api_key": "sk-...",
  "provider_slug": "anthropic",
  "has_quota_limit": 0,
  "description": "username_anthropic",
  "biz_date": "20260705"
}
```

平台返回字符串 `id`，KeyHub 会稳定映射为本地数字发布 ID。消费同步读取 `GET /api/apikey/list` 的 `actual_cost`，并按 `KEYHUB_NEWAPI_QUOTA_PER_USD` 转换为累计 quota。

## 上线流程

1. 在 KeyHub 导入或维护秘钥库存。
2. 在“我的秘钥”中点击“管理”，选择“上线”。
3. 选择一个已启用的接收平台并确认。
4. KeyHub 解密本地秘钥，转换成 new-api channel 参数并推送。
5. new-api 返回 channel 结果后，KeyHub 记录上线绑定关系。

## 秘钥形态兼容

- 用户不需要查看或选择具体平台；关联多个平台时只选择去重后的渠道类型，每个非空行作为一条完整秘钥原文保存。
- 选择渠道类型后，上传页会读取任一个匹配接收平台的模型列表供用户编辑，同时仍隐藏具体平台信息。
- 用户未编辑模型列表时，每个接收平台继续使用自身配置的模型；用户编辑后，本次导入创建的所有匹配上游渠道都使用编辑后的模型列表。
- 本次所选渠道类型包含旧版 AWS Bedrock 时，用户必须选择 `AccessKey / SecretAccessKey` 或 `API Key` 密钥类型。前者按 `AccessKey|SecretAccessKey|Region` 校验，后者按 `APIKey|Region` 校验。
- 新版 AWS Bedrock 与其他非旧版 AWS Bedrock 渠道不检查秘钥前缀、长度或供应商格式，也不会拆分 `|` 分隔的内容。
- 选择新版 AWS Bedrock 时必须填写至少一行区域地址，每行必须以 `https://bedrock-mantle` 开头。
- 新版 AWS Bedrock 按“区域 × 秘钥”展开入库；例如 2 个区域和 3 个秘钥会创建 6 条记录并向每个匹配的接收平台创建 6 个渠道。
- 使用 `new-api 逆向`接收平台创建新版 AWS Bedrock 渠道时，区域地址会作为渠道的 `base_url` 参数。
- 创建新版 AWS Bedrock 的 new-api 渠道时会自动生成 `model_mapping` JSON 字符串，将最终模型列表中的每个模型映射到 `anthropic.${模型ID}`。
- 上线时使用接收平台配置的渠道类型生成 new-api `type`，并把该平台手动配置的模型列表和秘钥原文写入渠道参数。
- 旧版 AWS Bedrock 使用 new-api `type=33`，并把用户选择转换为 `settings.aws_key_type`。
- 新版 AWS Bedrock 使用 Anthropic 的 new-api `type=14` 和 payload 结构，不写入 AWS 设置；渠道名称使用 `${用户名}_aws_bedrock_new`。
- 渠道名称为 `${用户名}_${渠道类型}`。
- 除新版 AWS Bedrock 的 `new-api 逆向`渠道外，渠道同步不发送 `base_url`，接收平台按 `type` 使用对应渠道类型的内置地址。

## 常见问题

- 如果提示 `401` 或鉴权失败，请确认 KeyHub 平台 Token 与 new-api `KEYHUB_PUSH_TOKEN` 完全一致。
- 如果新增平台提示必须实现 `GET /api/keyhub/channels/usage`，说明接收平台版本过旧或返回格式不符合 KeyHub 契约，需要先升级接收平台。
- 如果提示 `decrypt ciphertext`，通常是 `KEYHUB_ENCRYPTION_KEY` 变更导致旧数据无法解密，需要恢复原来的密钥或重新导入数据。
- 如果上线后 new-api 看不到可用渠道，请确认 channel 默认是禁用状态，需要在 new-api 中人工启用。
