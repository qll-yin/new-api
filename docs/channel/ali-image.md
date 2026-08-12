# 阿里通义渠道图片能力说明（Ali / ChannelTypeAli = 17）

本文档说明 `Ali(17)` 渠道当前已经落地的图片生成与图片编辑能力，重点回答两件事：

- `wan` 系列图片模型能否走 `new-api` 的 OpenAI 兼容图片接口
- `qwen` 系列图片模型能否走同一套接口

结论先写在前面：

- 可以。
- `Ali(17)` 对外走的是 OpenAI 兼容图片接口。
- `Ali(17)` 对内会再转换成阿里原生接口请求。
- 这和 `AliBailian(58)` 的任务型视频渠道不是同一种接入方式。

## 1. 对外接口

`Ali(17)` 图片能力当前走的是 `new-api` 统一图片入口：

- `POST /v1/images/generations`
- `POST /v1/images/edits`

对应代码：

- `router/relay-router.go`

这意味着前端或客户端侧不需要按百炼原生图片接口自己组装请求体，直接按 OpenAI 兼容格式请求即可。

## 2. 对内转发方式

`Ali(17)` 不是“客户端直传百炼原生 JSON”的渠道。

它的实际行为是：

1. 客户端请求 `new-api` 的 OpenAI 兼容图片接口
2. `Ali(17)` 适配器接管请求
3. 按模型类型改写为阿里原生图片接口
4. 再把上游响应转换回 `new-api` 统一格式

对应代码：

- `relay/channel/ali/adaptor.go`
- `relay/channel/ali/image.go`
- `relay/channel/ali/image_wan.go`

## 3. 当前已落地的图片模型

### 3.1 明确写在 `Ali(17)` 模型列表中的图片模型

以下模型已明确出现在 `relay/channel/ali/constants.go` 的 `ModelList` 中：

- `wan2.6-image`
- `wan2.6-t2i`
- `wan2.7-image`
- `wan2.7-image-pro`

### 3.2 通过规则匹配支持的同步图片模型

`Ali(17)` 的图片逻辑并不只看 `ModelList`，还会通过 `setting/model_setting/qwen.go` 的 `SyncImageModels` 做匹配判断。

当前默认同步图片模型规则包括：

- `z-image`
- `qwen-image`
- `wan2.6`
- `wan2.7`
- `qwen-image-edit`
- `qwen-image-edit-max`
- `qwen-image-edit-max-2026-01-16`
- `qwen-image-edit-plus`
- `qwen-image-edit-plus-2025-12-15`
- `qwen-image-edit-plus-2025-10-30`

这意味着：

- `qwen-image` 系列可以走 `POST /v1/images/generations`
- `qwen-image-edit*` 系列可以走 `POST /v1/images/edits`
- `wan2.6*` / `wan2.7*` 图片模型也可以走这两套 OpenAI 兼容图片接口

需要注意：

- `qwen-image` / `qwen-image-edit*` 当前主要是通过适配逻辑和模型匹配规则支持
- 它们并没有完整列在 `relay/channel/ali/constants.go` 的 `ModelList` 里
- 也就是说，“路由与适配支持”和“渠道模型列表展示”是两层逻辑，当前两者并不完全一致

## 4. 接口与模型对应关系

### 4.1 文生图

对外接口：

- `POST /v1/images/generations`

内部转发规则：

- 如果模型命中同步图片规则：
  - 转发到 `/api/v1/services/aigc/multimodal-generation/generation`
- 如果模型未命中同步图片规则：
  - 转发到 `/api/v1/services/aigc/text2image/image-synthesis`

因此当前可以明确认为支持这类调用的模型分组包括：

- `qwen-image*`
- `wan2.6-image`
- `wan2.6-t2i`
- `wan2.7-image`
- `wan2.7-image-pro`
- `z-image*`

### 4.2 图编辑

对外接口：

- `POST /v1/images/edits`

内部转发规则：

- 旧版 `wan` 图片编辑模型：
  - 转发到 `/api/v1/services/aigc/image2image/image-synthesis`
- `wan2.6*` / `wan2.7*` 图片模型：
  - 转发到 `/api/v1/services/aigc/image-generation/generation`
- 其他编辑类模型：
  - 转发到 `/api/v1/services/aigc/multimodal-generation/generation`

因此当前可以明确认为支持这类调用的模型分组包括：

- `qwen-image-edit*`
- `wan2.6*` 图片模型
- `wan2.7*` 图片模型
- 部分旧版 `wan` 编辑模型

其中 `wan` 编辑模型有专门转换逻辑，位于：

- `relay/channel/ali/image_wan.go`

## 5. 同步 / 异步行为

### 5.1 文生图

- 命中同步图片规则的模型：
  - 走同步图片逻辑
- 未命中同步图片规则的模型：
  - 请求头会附带 `X-DashScope-Async: enable`
  - 走异步图片任务逻辑

### 5.2 图编辑

- `wan2.6*` / `wan2.7*` 编辑虽然命中同步模型规则，但适配器会强制按异步方式处理
- 代码里会给这类编辑请求设置 `X-DashScope-Async: enable`

这也是为什么 `wan` 图片生成和 `wan` 图片编辑在内部路径和任务处理上并不完全相同。

## 6. 请求体兼容方式

### 6.1 文生图

`Ali(17)` 接收的仍是标准 OpenAI 图片生成请求，例如：

```json
{
  "model": "wan2.7-image",
  "prompt": "画一只猫",
  "n": 2,
  "size": "1024x1024"
}
```

适配器会转换为阿里图片请求结构。

### 6.2 图编辑

`POST /v1/images/edits` 支持两种进入方式：

- `multipart/form-data`
- JSON 请求体

其中：

- `qwen-image-edit*` 这类编辑模型会走通用阿里图片编辑转换逻辑
- `wan` 编辑模型会走单独的 `wan` 编辑转换逻辑

## 7. 计费要点

`Ali(17)` 图片计费当前有一个很关键的行为：

- 当请求里传了 `n`
- 适配器会把 `n` 写入 `PriceData.OtherRatios`
- 最终按图片张数参与计费

因此如果后台把某图片模型配置为按次 `0.035`，且请求参数为：

```json
{
  "model": "wan2.7-image",
  "prompt": "test",
  "n": 3
}
```

则最终会按 `0.035 x 3 = 0.105` 计算，而不是只收一次 `0.035`。

对应代码：

- `relay/channel/ali/image.go`
- `relay/image_handler.go`
- `service/text_quota.go`

## 8. 与 `AliBailian(58)` 的区别

建议按当前项目分工理解：

- `Ali(17)`：
  - 负责阿里图片链路
  - 对外提供 OpenAI 兼容图片接口
  - 对内转换到阿里原生图片接口
- `AliBailian(58)`：
  - 负责百炼任务型视频能力
  - 重点是异步视频任务，不是图片主通道

因此从现有代码实现看：

- `wan` 系列图片模型更适合放在 `Ali(17)`
- `wan` 系列视频模型更适合放在 `AliBailian(58)`

## 9. 当前可以直接给出的结论

### 9.1 `wan` 系列生图是否可以走 OpenAI 图片接口

可以。

当前已确认可走 `new-api` 图片接口的 `wan` 图片模型至少包括：

- `wan2.6-image`
- `wan2.6-t2i`
- `wan2.7-image`
- `wan2.7-image-pro`

使用入口：

- `POST /v1/images/generations`
- `POST /v1/images/edits`

### 9.2 `qwen` 系列生图是否可以走 OpenAI 图片接口

可以。

当前已确认支持：

- `qwen-image*` 走 `POST /v1/images/generations`
- `qwen-image-edit*` 走 `POST /v1/images/edits`

### 9.3 `Ali(17)` 是否等同于“客户端直接按百炼原生格式请求”

不等同。

`Ali(17)` 是：

- 对外 OpenAI 兼容
- 对内阿里原生转换

而不是把客户端请求原样透传给百炼原生图片接口。

## 10. 相关代码位置

- 路由入口：`router/relay-router.go`
- Ali 图片适配：`relay/channel/ali/adaptor.go`
- Ali 图片请求转换：`relay/channel/ali/image.go`
- Wan 图片编辑转换：`relay/channel/ali/image_wan.go`
- Ali 模型列表：`relay/channel/ali/constants.go`
- 同步图片模型配置：`setting/model_setting/qwen.go`

