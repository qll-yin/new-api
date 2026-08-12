# 原版二开新增功能清单

更新时间：2026-06-14

本文档用于整理当前仓库相对官方原版 `new-api` 已经落地的二开功能与模块，重点覆盖目前已经写入代码、前后端可见、并且与你当前视频中转场景直接相关的部分。

说明：

- 本文档描述的是“当前这个原版二开仓库里已经存在的能力”，不是设想中的功能。
- 官方 upstream 自身后续新增的功能不计入本清单。
- 重点聚焦阿里云百炼 HappyHorse、通用视频计费、异步任务计费与日志链路。

## 1. 阿里云百炼视频渠道接入

当前仓库已经新增了阿里云百炼视频任务渠道，核心目标是把 DashScope 的异步视频任务接进 `new-api` 的统一任务管线。

已落地内容：

- 新增渠道类型 `AliBailian`，用于百炼视频任务接入。
- 新增 `relay/channel/task/happyhorse/` 适配器，实现 HappyHorse 视频模型的任务提交、轮询查询、状态映射与结果转换。
- 支持在 `/v1/models` 和渠道模型列表中暴露 HappyHorse 系列模型。
- 支持 DashScope 原生风格入口和 `new-api` 自身统一入口并存。

当前支持的调用方式：

- DashScope 原生视频接口：
  - `POST /api/v1/services/aigc/video-generation/video-synthesis`
  - `GET /api/v1/tasks/{task_id}`
- `new-api` 统一视频接口：
  - `POST /v1/video/generations`
  - `GET /v1/video/generations/{task_id}`
- OpenAI 风格视频接口：
  - `POST /v1/videos`
  - `GET /v1/videos/{task_id}`

相关文件：

- `relay/channel/task/happyhorse/adaptor.go`
- `middleware/happyhorse_adapter.go`
- `controller/model.go`
- `relay/relay_adaptor.go`
- `docs/channel/alibailian.md`

## 2. HappyHorse 原生格式兼容

为了兼容阿里云百炼原生调用方式，当前仓库已经补了请求和查询中间件，把百炼原生请求自动收敛到内部统一任务逻辑，再按调用场景返回原生结构。

已落地内容：

- 提交请求中间件会把 DashScope 原生请求体转换成内部统一视频任务格式。
- 查询请求中间件会把 `GET /api/v1/tasks/:task_id` 重写到内部查询逻辑。
- 原生返回中的 `task_id` 对外暴露为平台内部任务 ID，避免用户拿到上游真实 ID 后无法在本平台查询。
- 同时保留上游 `request_id` / 上游任务 ID，用于内部映射、调试和日志排查。

这部分的意义是：

- 对客户端保持百炼原生协议兼容。
- 对平台内部保持统一计费、统一日志、统一轮询、统一状态同步。

## 3. 通用视频计费配置

当前仓库已经不再把视频计费写死为某个模型的临时逻辑，而是补了一套通用视频计费配置能力，后续可以复用到 HappyHorse、Seedance、Kling、Wan 等其他视频模型。

已落地内容：

- 新增 `VideoModelConfig` 配置结构。
- 支持为模型设置：
  - 基础分辨率 `base_resolution`
  - 分辨率倍率 `resolution_multipliers`
- 支持把 `720 / 1080 / 480` 等输入统一规范化为 `720P / 1080P / 480P`。
- 支持按模型读取视频计费配置，并在未传分辨率时回退到基础分辨率。

当前视频计费含义：

- `model_price` 表示“基础分辨率下，每秒基础单价”。
- 实际价格按“秒数 × 分辨率倍率 × 分组倍率”计算。
- 若模型没有额外分辨率倍率，则基础分辨率按 `1x` 计算。

相关文件：

- `setting/ratio_setting/video_billing.go`
- `setting/ratio_setting/model_ratio.go`
- `relay/channel/task/taskcommon/video_billing.go`

## 4. 视频请求参数与价格联动

当前仓库已经把视频价格与请求参数打通，而不是只做固定按次计费。

已落地内容：

- 任务提交时会从视频请求中提取时长和分辨率。
- 计费阶段会生成通用的附加倍率键，例如：
  - `seconds`
  - `resolution-720P`
  - `resolution-1080P`
- 计费系统会根据这些键计算预扣额度。
- 轮询结算阶段可以再次根据真实结果或任务上下文重算费用。

这意味着当前视频计费是“参数驱动”的，而不是“单纯固定一次多少钱”。

## 5. 异步任务计费链路完善

当前仓库已经补齐了异步视频任务从“提交预扣”到“任务失败退款”再到“完成差额结算”的完整链路。

已落地内容：

- 任务提交时写入异步任务消费日志。
- 任务私有数据中保存计费快照 `TaskBillingContext`，用于轮询阶段重新结算。
- 任务失败时自动退款。
- 退款不仅回退用户余额，也会同步回退令牌额度。
- 若任务通过订阅计费，也支持对订阅额度做退款或差额调整。
- 若最终实际费用与预扣费用不同，支持差额补扣或差额返还。

当前支持的资金来源：

- 钱包余额
- 订阅额度
- Token 配额

相关文件：

- `service/task_billing.go`
- `service/task_polling.go`
- `model/task.go`
- `model/user.go`

## 6. 异步任务状态与日志增强

当前仓库已经补充了异步视频任务的状态同步和日志回写能力，避免“任务失败了，但日志还是普通消费记录”的问题。

已落地内容：

- 日志 `other` 中写入：
  - `is_task`
  - `task_id`
  - `task_status`
  - `task_progress`
  - `upstream_task_id`
  - `reason`
  - `result_url`
- 任务状态同步时，会把对应请求日志更新为最新状态。
- 当任务失败时，原消费日志可转为错误日志。
- 退款记录会单独生成退款类日志。

目前日志类型语义：

- `type = 2`：消费
- `type = 5`：错误
- `type = 6`：退款

这样做的好处是：

- 用户能看到任务是否成功、失败、排队中、处理中。
- 管理员能区分“创建任务成功但后续失败”与“计费退款记录”。
- 后续对账和排查更清晰。

## 7. Request ID 与 Upstream Request ID 拆分

当前仓库已经明确区分“本系统请求编号”和“上游平台请求编号”，避免视频异步任务在调试时混淆。

字段含义：

- `request_id`：本系统内的请求 ID
- `upstream_request_id`：上游厂商返回的请求 ID

已落地内容：

- 任务私有数据中保存 `RequestID` 和 `UpstreamRequestID`
- 日志模型中支持 `UpstreamRequestId`
- 百炼 / HappyHorse 响应转换时会把上游 request id 保留下来

相关文件：

- `model/log.go`
- `model/task.go`
- `relay/channel/task/happyhorse/adaptor.go`
- `relay/channel/task/ali/adaptor.go`

## 8. 模型定价接口增强

当前仓库已经把视频计费信息接入到定价模型结构中，不再只是文本模型的倍率或按次价格。

已落地内容：

- `Pricing` 结构中新增 `VideoModelConfig`
- 模型定价接口可以向前端返回视频计费配置
- 模型广场和管理后台能够识别“该模型属于视频计费模式”

相关文件：

- `model/pricing.go`
- `model/option.go`
- `setting/ratio_setting/exposed_cache.go`

## 9. 管理后台新增视频计费编辑能力

当前仓库已经在管理后台增加视频计费相关配置，不再只能设置按量、按次、阶梯表达式。

已落地内容：

- 模型定价编辑器新增视频计费模式。
- 可配置基础分辨率与分辨率倍率。
- 模型列表与定价表格已兼容视频计费展示。
- 模型广场可识别视频计费标签与视频价格摘要。

已覆盖前端主题：

- `classic`
- `default`

相关文件：

- `web/classic/src/pages/Setting/Ratio/components/ModelPricingEditor.jsx`
- `web/classic/src/pages/Setting/Ratio/ModelRatioSettings.jsx`
- `web/classic/src/components/table/model-pricing/...`
- `web/default/src/features/system-settings/models/...`

## 10. 使用日志前端展示增强

当前仓库已经针对异步视频任务增强了使用日志展示，尤其是 `classic` 主题。

已落地内容：

- 列表页可直接看到任务状态摘要。
- 展开详情可看到：
  - `Task ID`
  - `Upstream Task ID`
  - `Status`
  - `Progress`
  - `Reason / 任务失败原因 / 退款原因`
- 任务失败、退款、状态同步这几类信息都能在使用日志里定位。

这部分主要解决的是：

- 用户不知道异步视频任务当前是否还在排队。
- 用户不知道失败任务对应哪个 task。
- 管理员需要根据任务 ID 和失败原因排查上游问题。

相关文件：

- `web/classic/src/components/table/usage-logs/UsageLogsColumnDefs.jsx`
- `web/classic/src/hooks/usage-logs/useUsageLogsData.jsx`
- `web/default/src/features/usage-logs/...`

## 11. 多语言补充

当前仓库已经为新增的视频计费、任务状态、日志展示补充了多语言键，覆盖现有前端多语言文件。

已处理语言：

- `zh`
- `en`
- `fr`
- `ja`
- `ru`
- `vi`
- `classic` 主题额外的 `zh-CN / zh-TW`

相关目录：

- `web/classic/src/i18n/locales/`
- `web/default/src/i18n/locales/`

## 12. 测试与文档补充

当前仓库已经补充了与本次二开相关的部分测试和文档，便于后续继续扩展。

已落地内容：

- HappyHorse 适配器测试
- HappyHorse 中间件测试
- 通用视频计费测试
- 异步任务计费测试
- 阿里云百炼渠道开发文档

相关文件：

- `relay/channel/task/happyhorse/adaptor_test.go`
- `middleware/happyhorse_adapter_test.go`
- `relay/channel/task/taskcommon/video_billing_test.go`
- `setting/ratio_setting/video_billing_test.go`
- `service/task_billing_test.go`
- `docs/channel/alibailian.md`

## 13. 当前这套二开的核心价值

相对官方原版，目前这套二开最核心的新增价值主要集中在下面几类：

1. 把阿里云百炼 HappyHorse 视频任务真正接进了 `new-api`。
2. 把视频计费从简单按次，推进到了按秒、按分辨率、按分组倍率联动。
3. 把异步任务从“只能提交和轮询”推进到了“可预扣、可退款、可差额结算、可写日志、可展示状态”。
4. 把前端后台从“只能配文本计费”推进到了“可配置通用视频计费”。
5. 为后续新增其他视频模型预留了可复用的通用计费和日志结构。

## 14. 目前最适合继续扩展的方向

如果你后续继续在这套仓库上迭代，当前最适合沿用现有设计继续扩展的方向是：

1. 在通用视频计费配置下继续接入新的视频模型，例如 Seedance、Kling、Wan。
2. 继续沿用 `task_status / task_id / upstream_task_id / reason` 这套日志字段做异步任务标准化。
3. 针对不同上游任务平台，继续补充更多“原生格式兼容入口”。
4. 在模型广场和管理后台进一步细化视频价格展示，例如多分辨率价格摘要。

## 15. 备注

如果后续你希望把这份文档继续细化，我建议可以再拆成两份：

- 一份“面向管理员”的功能说明，强调如何配置视频模型、如何配置价格、怎么看日志。
- 一份“面向开发”的技术设计文档，强调任务链路、计费链路、状态同步、日志字段与扩展点。

