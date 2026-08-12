# 阿里云百炼（AliBailian / DashScope）渠道说明

本文档描述 `new-api` 中 `AliBailian(58)` 渠道的当前实现、万相系列支持范围、与 `Ali(17)` 的职责分工，以及任务计费与失败退款行为。

## 1. 当前定位

- 渠道类型 ID：`58`（`constant.ChannelTypeAliBailian`）
- 渠道名称：`AliBailian`
- 任务平台：`"58"`
- 适配器目录：`relay/channel/task/happyhorse/`
- 上游类型：DashScope 异步任务接口
- 认证方式：
  - `Authorization: Bearer <key>`
  - `X-DashScope-Async: enable`
- BaseURL：
  - 需要配置百炼工作空间对应域名
  - 例如 `https://{WorkspaceId}.ap-southeast-1.maas.aliyuncs.com`

说明：
- 目录名仍为 `happyhorse`，但它现在表示“百炼视频任务适配器”，不再只服务 `happyhorse-1.0-*` 模型。

## 2. 渠道职责分工

当前项目对阿里系模型采用如下分流：

- `Ali(17)`：承载阿里通义图片模型与同步图片能力
- `AliBailian(58)`：承载阿里云百炼异步视频任务能力

这样拆分的原因：

- 万相图片模型与现有阿里图片链路兼容性更高
- 万相视频模型属于 DashScope 任务型接口，更适合复用百炼异步任务通道
- 可以尽量减少对上游 `new-api` 原有 `Ali(17)` 逻辑的侵入，降低后续合并冲突

## 3. 万相模型支持范围

### 3.1 `Ali(17)` 中承载的万相图片模型

这些模型不放在 `AliBailian(58)`，而是继续走阿里图片链路：

- `wan2.6-image`
- `wan2.6-t2i`
- `wan2.7-image`
- `wan2.7-image-pro`

相关文件：

- `relay/channel/ali/constants.go`
- `relay/channel/ali/image.go`
- `relay/channel/ali/image_wan.go`

### 3.2 `AliBailian(58)` 中承载的视频模型

当前已接入：

- `happyhorse-1.0-t2v`
- `happyhorse-1.0-i2v`
- `happyhorse-1.0-r2v`
- `happyhorse-1.0-video-edit`
- `wan2.7-t2v`
- `wan2.7-i2v-2026-04-25`
- `wan2.7-r2v`
- `wan2.7-videoedit`
- `wan2.2-animate-move`
- `wan2.2-animate-mix`

相关文件：

- `relay/channel/task/happyhorse/constants.go`
- `relay/channel/task/happyhorse/adaptor.go`

## 4. 上游接口映射

### 4.1 `happyhorse-1.0-*` 与 `wan2.7-*`

适用模型：

- `happyhorse-1.0-*`
- `wan2.7-*`

上游提交接口：

- `POST /api/v1/services/aigc/video-generation/video-synthesis`

上游查询接口：

- `GET /api/v1/tasks/{task_id}`

特点：

- 异步任务
- 支持 `t2v / i2v / r2v / videoedit`
- 内部统一转换为 `HappyHorseRequest`

### 4.2 `wan2.2-animate-*`

适用模型：

- `wan2.2-animate-move`
- `wan2.2-animate-mix`

上游提交接口：

- `POST /api/v1/services/aigc/image2video/video-synthesis`

上游查询接口：

- `GET /api/v1/tasks/{task_id}`

特点：

- 仍然是异步任务
- 请求体沿用旧结构：`input.image_url`、`input.video_url`、`input.watermark`
- 成功结果可能出现在 `output.results.video_url`

## 5. 对外兼容的三种入口

### 5.1 DashScope 原生接口

提交：

- `POST /api/v1/services/aigc/video-generation/video-synthesis`
- `POST /api/v1/services/aigc/image2video/video-synthesis`

查询：

- `GET /api/v1/tasks/{task_id}`

用途：

- 兼容原生 DashScope 客户端
- 客户端通常只需替换 `base_url` 与 `api_key`

### 5.2 new-api 统一任务接口

- `POST /v1/video/generations`
- `GET /v1/video/generations/{task_id}`

### 5.3 OpenAI Video 兼容接口

- `POST /v1/videos`
- `GET /v1/videos/{task_id}`

说明：

- OpenAI Video 查询结果由适配器转换为统一视频任务格式
- DashScope 原生查询结果会保留原生结构，但会将公开 `task_id` 重写为平台内任务 ID

## 6. 中间件与路由改写

相关文件：

- `router/video-router.go`
- `middleware/happyhorse_adapter.go`

行为说明：

- DashScope 原生提交请求先经过 `HappyHorseRequestConvert()`
- 中间件会把原生请求体改写为内部统一任务结构，再复用 `/v1/video/generations`
- DashScope 原生查询先经过 `HappyHorseFetchConvert()`
- 中间件会把 `/api/v1/tasks/:task_id` 改写到内部统一任务查询链路
- 对于 `wan2.2-animate-*`：
  - 中间件会保留 `metadata.input`
  - 这样适配器仍可还原旧版 `input.image_url / input.video_url` 提交格式

## 7. 模型映射兼容

当前适配器已经兼容渠道模型映射。

这意味着：

- 如果后台配置了模型映射
- 例如自定义别名映射到 `wan2.7-i2v-2026-04-25`
- 或映射到 `wan2.2-animate-move`

则请求校验会按映射后的上游真实模型规则执行，而不是按别名本身执行。

这样可以避免以下错误：

- 图生视频模型本来允许不传 `prompt`，却因为别名不含 `i2v` 被误判
- `wan2.2-animate-*` 本来应校验 `metadata.input`，却因为别名不含 `wan2.2-animate-` 被误判

相关文件：

- `relay/channel/task/happyhorse/adaptor.go`
- `relay/channel/task/happyhorse/adaptor_test.go`

## 8. 计费逻辑

### 8.1 未定价模型的行为

保持 `new-api` 官方逻辑不变：

- 如果模型没有配置价格或倍率
- 直接报“模型未定价”

这里不做特殊放宽。

### 8.2 视频任务预估计费

对于以下模型：

- `happyhorse-1.0-*`
- `wan2.7-*`

适配器会在任务提交前根据请求参数估算 `OtherRatios`，主要包括：

- 秒数
- 分辨率

这些倍率会参与预扣费。

### 8.3 `wan2.2-animate-*` 的计费特点

对于以下模型：

- `wan2.2-animate-move`
- `wan2.2-animate-mix`

当前实现重点是兼容官方任务接口：

- 提交前不主动生成基于秒数/分辨率的预估倍率
- `EstimateBilling()` 返回 `nil`
- 失败时仍会正常退款
- 成功后保持现有结算链路，不额外强加自定义估算逻辑

### 8.4 默认价格配置

默认模型价格与视频倍率兜底已补入：

- `setting/ratio_setting/model_ratio.go`

但这只是默认值。

线上仍建议通过后台“模型定价”统一配置，避免后续价格调整时依赖代码变更。

## 9. 失败任务退款与日志行为

相关文件：

- `service/task_billing.go`
- `service/task_polling.go`

当前失败任务采用“单条任务日志回写”模式：

1. 用户提交任务后，先记录一条 `consume` 类型任务日志
2. 如果任务最终失败：
   - 将预扣额度退回用户钱包或订阅额度
   - 将令牌额度补回
   - 回滚用户 `used_quota`
   - 回滚渠道 `used_quota`
   - 将原来的那条 `consume` 日志改写为 `error`
   - 原日志 `quota` 归零
   - `other` 中补入：
     - `pre_consumed_quota`
     - `actual_quota=0`
     - `refunded_quota`
     - `reason`
3. 不再为失败任务额外插入一条新的 `refund` 日志

这样可以避免：

- 日志里出现“失败任务仍被统计为已消费”
- 同一失败任务同时出现 `consume + refund` 两条记录，影响账务观察

补充说明：

- 成功任务如果在“预扣费”和“最终实际扣费”之间有差额，仍可能出现独立的 `refund` 或 `consume` 结算日志
- 这属于成功任务的差额结算，不属于失败任务退款

## 10. 当前测试覆盖

本次相关链路已经补充并验证了以下场景：

### 10.1 百炼适配器

- `wan2.7-i2v-2026-04-25` 无 `prompt` 校验
- `wan2.2-animate-*` 无 `prompt` 校验
- 模型映射到 `wan2.7-i2v-*` 的校验兼容
- 模型映射到 `wan2.2-animate-*` 的校验兼容
- `wan2.7-r2v` 请求参数转换
- `wan2.7-videoedit` 省略 `duration/ratio`
- `wan2.2-animate-*` 旧版请求体转换
- `wan2.2-animate-*` 查询结果 `output.results.video_url` 解析
- DashScope 原生响应 `task_id` 重写

### 10.2 中间件

- DashScope 原生请求改写到统一任务接口
- `wan2.2-animate-*` 的 `metadata.input` 保留

### 10.3 失败退款

- 失败任务把原 `consume` 日志改写为 `error`
- 钱包额度补回
- 令牌额度补回
- 用户 `used_quota` 回滚
- 渠道 `used_quota` 回滚
- CAS 场景下避免重复退款

## 11. 建议的新增模型扩展方式

### 11.1 新增百炼视频模型

建议优先这样做：

- 先把模型名加入 `relay/channel/task/happyhorse/constants.go`
- 如果请求结构兼容现有 `wan2.7-*` 或 `wan2.2-animate-*`
  - 尽量复用现有分支
- 只有在请求结构明显不同的情况下
  - 再在 `adaptor.go` 中按模型分支扩展

### 11.2 新增万相图片模型

当前仍建议：

- 优先放入 `Ali(17)` 图片链路
- 不建议塞进 `AliBailian(58)` 任务通道

原因：

- 图片接口是同步调用
- 不适合复用当前异步任务适配器
- 更利于与 upstream `new-api` 保持兼容

## 12. 验证命令

建议使用如下命令进行回归验证：

```bash
go test ./relay/channel/task/happyhorse ./middleware ./setting/ratio_setting
go test ./service
go build ./...
```

如果只想验证这次百炼/万相改动的关键链路，可优先执行：

```bash
go test ./relay/channel/task/happyhorse -run "ValidateRequestAndSetAction|BuildRequestURL|ConvertTo|EstimateBilling|AdjustBilling|ParseTaskResult|ConvertToDashScopeNative"
go test ./service -run "RefundTaskQuota|CASGuardedRefund|Settle|ObserveChannelAffinityUsageCacheByRelayFormat"
```
