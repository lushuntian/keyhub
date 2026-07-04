# 第三步计划：同步 new-api 渠道

第二步已经完成 Key 上传、预览、去重、加密入库和库存列表。第三步开始把 KeyHub 的库存转成 `new-api` 渠道。

## 目标

- 实现 KeyHub 到 `new-api` 的渠道创建。
- 支持从库存 Key 手动上线为活跃渠道。
- 记录 `newapi_channel_id`、同步状态和同步事件。
- 为后续自动补池打基础。

## 后端接口

- `POST /api/keys/{id}/activate`：把单个库存 Key 同步到 new-api，并标记为 active。
- `POST /api/sync/new-api`：按条件批量同步。
- `GET /api/sync/events`：查看同步事件。
- `POST /api/keys/{id}/disable`：禁用 KeyHub 记录，并同步禁用 new-api 渠道。

## new-api 对接

优先调用 `new-api` 管理 API：

- `POST /api/channel/`
- `PUT /api/channel/`
- `DELETE /api/channel/{id}`

同步 payload 需要从 KeyHub 记录生成：

- `type`：来自 `category_pool_rules.newapi_type`
- `key`：解密后的上游 key
- `name`：按分类、标签和 key hint 生成
- `models`：KeyHub 里的模型数组转逗号字符串
- `group`：KeyHub 的 `group_name`
- `tag`：KeyHub 的标签
- `base_url`：Azure 等渠道使用
- `auto_ban`：默认开启

## 完成标准

- 在页面上点击某个库存 Key，可以创建 new-api 渠道。
- KeyHub 能记录同步成功/失败。
- 同步成功后控制台活跃数变化。
- 不在日志和接口响应中泄露完整 key。
