# 第二步计划：上传 Key 与入库

第一步完成项目骨架和基础数据模型后，第二步进入核心业务的第一条闭环：上传供应商 Key，并进入库存池。

## 目标

- 实现批量上传和单个上传 API。
- 支持分类、标签、分组、模型范围、备注、是否先入库存。
- 对 key 做格式解析、指纹去重、脱敏展示。
- 原始 key 加密入库。
- 前端完成“上传密钥”页面。

## 后端接口

- `POST /api/keys/preview`：解析输入，返回行级预览、错误、脱敏 key。
- `POST /api/keys/import`：确认导入，写入 `key_import_batches` 和 `api_keys`。
- `GET /api/keys`：分页查询 key，默认不返回原始密钥。
- `GET /api/categories`：返回分类、默认模型和 new-api 类型。

## 关键实现

- AWS Bedrock：解析 `AccessKey|SecretKey|Region` 和 `ApiKey|Region`。
- OpenAI/Anthropic/Gemini：解析单 key。
- Azure OpenAI：解析 endpoint、api key、api version。
- 指纹：`sha256(category + normalized key)`。
- 加密：AES-GCM，密钥来自 `KEYHUB_ENCRYPTION_KEY`。
- 审计：导入成功后写 `audit_logs`。

## 完成标准

- Docker Compose 环境中可上传一批 AWS Bedrock key。
- 重复 key 不会重复入库。
- 页面能看到导入批次和库存数量变化。
