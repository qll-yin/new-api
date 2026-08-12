package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func effectiveTaskModelPrice(modelPrice float64, resolvedModelPrice float64, otherRatios map[string]float64) float64 {
	if resolvedModelPrice > 0 {
		return resolvedModelPrice
	}
	return taskcommon.ResolveVideoModelPrice(modelPrice, otherRatios)
}

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	if info.PriceData.Quota == 0 {
		return
	}
	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	// 支持任务仅按次计费
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if otherRatios := info.PriceData.OtherRatios(); len(otherRatios) > 0 {
			var contents []string
			for key, ra := range otherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.6f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	other["is_task"] = true
	other["task_id"] = info.PublicTaskID
	other["task_status"] = string(model.TaskStatusSubmitted)
	other["request_path"] = c.Request.URL.Path
	otherRatios := info.PriceData.OtherRatios()
	other["model_price"] = effectiveTaskModelPrice(info.PriceData.ModelPrice, info.PriceData.ResolvedModelPrice, otherRatios)
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if len(otherRatios) > 0 {
		other["other_ratios"] = otherRatios
	}
	for key, value := range otherRatios {
		other[key] = value
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	attachQuotaSaturation(c, info, other)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     info.UsingGroup,
		Other:     other,
	})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
	model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
}

// LogTaskCreateFailure records a single task-style error log when task creation
// has entered the upstream submission flow but eventually fails.
func LogTaskCreateFailure(c *gin.Context, info *relaycommon.RelayInfo, taskErr *dto.TaskError) {
	if c == nil || info == nil || taskErr == nil || taskErr.LocalError {
		return
	}
	if info.Billing == nil && !info.PriceData.FreeModel {
		return
	}

	tokenName := c.GetString("token_name")
	reason := taskErr.Message
	if reason == "" && taskErr.Error != nil {
		reason = taskErr.Error.Error()
	}
	if reason == "" {
		reason = "task create failed"
	}

	other := make(map[string]interface{})
	other["is_task"] = true
	other["task_status"] = string(model.TaskStatusFailure)
	other["reason"] = reason
	if c.Request != nil && c.Request.URL != nil {
		other["request_path"] = c.Request.URL.Path
	}
	if info.PublicTaskID != "" {
		other["task_id"] = info.PublicTaskID
	}
	if info.Action != "" {
		other["task_action"] = info.Action
	}
	otherRatios := info.PriceData.OtherRatios()
	if resolvedPrice := effectiveTaskModelPrice(info.PriceData.ModelPrice, info.PriceData.ResolvedModelPrice, otherRatios); resolvedPrice > 0 {
		other["model_price"] = resolvedPrice
	}
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if len(otherRatios) > 0 {
		other["other_ratios"] = otherRatios
	}
	for key, value := range otherRatios {
		other[key] = value
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	if info.Billing != nil && info.PriceData.Quota > 0 {
		other["pre_consumed_quota"] = info.PriceData.Quota
		other["actual_quota"] = 0
		other["refunded_quota"] = info.PriceData.Quota
	}
	if taskErr.Code != "" {
		other["error_code"] = taskErr.Code
	}
	if taskErr.StatusCode > 0 {
		other["status_code"] = taskErr.StatusCode
	}

	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	useTimeSeconds := 0
	if !startTime.IsZero() {
		useTimeSeconds = int(info.StartTime.Sub(startTime).Seconds())
		if useTimeSeconds < 0 {
			useTimeSeconds = 0
		}
	}
	model.RecordErrorLog(
		c,
		info.UserId,
		info.ChannelId,
		info.OriginModelName,
		tokenName,
		reason,
		info.TokenId,
		useTimeSeconds,
		false,
		info.UsingGroup,
		other,
	)
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// resolveTokenKey 通过 TokenId 运行时获取令牌 Key（用于 Redis 缓存操作）。
// 如果令牌已被删除或查询失败，返回空字符串。
func resolveTokenKey(ctx context.Context, tokenId int, taskID string) string {
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("获取令牌 key 失败 (tokenId=%d, task=%s): %s", tokenId, taskID, err.Error()))
		return ""
	}
	return token.Key
}

// taskIsSubscription 判断任务是否通过订阅计费。
func taskIsSubscription(task *model.Task) bool {
	return task.PrivateData.BillingSource == BillingSourceSubscription && task.PrivateData.SubscriptionId > 0
}

// taskAdjustFunding 调整任务的资金来源（钱包或订阅），delta > 0 表示扣费，delta < 0 表示退还。
func taskAdjustFunding(task *model.Task, delta int) error {
	if taskIsSubscription(task) {
		return model.PostConsumeUserSubscriptionDelta(task.PrivateData.SubscriptionId, int64(delta))
	}
	if delta > 0 {
		return model.DecreaseUserQuota(task.UserId, delta, false)
	}
	return model.IncreaseUserQuota(task.UserId, -delta, false)
}

// taskAdjustTokenQuota 调整任务的令牌额度，delta > 0 表示扣费，delta < 0 表示退还。
// 需要通过 resolveTokenKey 运行时获取 key（不从 PrivateData 中读取）。
func taskAdjustTokenQuota(ctx context.Context, task *model.Task, delta int) {
	if task.PrivateData.TokenId <= 0 || delta == 0 {
		return
	}
	tokenKey := resolveTokenKey(ctx, task.PrivateData.TokenId, task.TaskID)
	if tokenKey == "" {
		return
	}
	var err error
	if delta > 0 {
		err = model.DecreaseTokenQuota(task.PrivateData.TokenId, tokenKey, delta)
	} else {
		err = model.IncreaseTokenQuota(task.PrivateData.TokenId, tokenKey, -delta)
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("调整令牌额度失败 (delta=%d, task=%s): %s", delta, task.TaskID, err.Error()))
	}
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = effectiveTaskModelPrice(bc.ModelPrice, bc.ResolvedModelPrice, bc.OtherRatios)
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		if bc.GroupRatio > 0 {
			other["group_ratio"] = bc.GroupRatio
		}
		if priceData := taskBillingContextPriceData(bc); priceData != nil {
			for k, v := range priceData.OtherRatios() {
				other[k] = v
			}
		}
	}
	modelName := taskModelName(task)
	if _, ok := other["model_ratio"]; !ok {
		other["model_ratio"], _, _ = ratio_setting.GetModelRatio(modelName)
	}
	if _, ok := other["group_ratio"]; !ok {
		other["group_ratio"] = ratio_setting.GetGroupRatio(task.Group)
	}
	other["completion_ratio"] = ratio_setting.GetCompletionRatio(modelName)
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

func taskBillingContextPriceData(bc *model.TaskBillingContext) *types.PriceData {
	if bc == nil || len(bc.OtherRatios) == 0 {
		return nil
	}
	priceData := &types.PriceData{}
	if !priceData.ReplaceOtherRatios(bc.OtherRatios) {
		return nil
	}
	return priceData
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

func taskLogStatus(task *model.Task) string {
	if task == nil {
		return ""
	}
	if len(task.Data) > 0 {
		var payload map[string]interface{}
		if err := common.Unmarshal(task.Data, &payload); err == nil {
			if output, ok := payload["output"].(map[string]interface{}); ok {
				if status, ok := output["task_status"].(string); ok && status != "" {
					return status
				}
			}
			if status, ok := payload["task_status"].(string); ok && status != "" {
				return status
			}
		}
	}
	return string(task.Status)
}

func syncTaskStateLog(ctx context.Context, task *model.Task) bool {
	if task == nil || task.PrivateData.RequestID == "" {
		return false
	}

	log, exist, err := model.GetLatestLogByRequestID(task.PrivateData.RequestID, []int{model.LogTypeConsume, model.LogTypeError})
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("get task lifecycle log failed (task=%s): %s", task.TaskID, err.Error()))
		return false
	}
	if !exist || log == nil {
		return false
	}

	other, _ := common.StrToMap(log.Other)
	if other == nil {
		other = make(map[string]interface{})
	}
	other["is_task"] = true
	other["task_id"] = task.TaskID
	other["task_status"] = taskLogStatus(task)
	if task.Progress != "" {
		other["task_progress"] = task.Progress
	}
	if upstreamTaskID := task.GetUpstreamTaskID(); upstreamTaskID != "" && upstreamTaskID != task.TaskID {
		other["upstream_task_id"] = upstreamTaskID
	}
	if task.FailReason != "" {
		other["reason"] = task.FailReason
	}
	if task.PrivateData.ResultURL != "" {
		other["result_url"] = task.PrivateData.ResultURL
	}
	if task.Status == model.TaskStatusFailure {
		if log.Quota > 0 {
			if _, ok := other["pre_consumed_quota"]; !ok {
				other["pre_consumed_quota"] = log.Quota
			}
			other["actual_quota"] = 0
			other["refunded_quota"] = log.Quota
			log.Quota = 0
		}
		log.Type = model.LogTypeError
	}
	log.Other = common.MapToJsonStr(other)
	if task.PrivateData.UpstreamRequestID != "" {
		log.UpstreamRequestId = task.PrivateData.UpstreamRequestID
	}
	if err = model.UpdateLog(log); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("update task lifecycle log failed (task=%s): %s", task.TaskID, err.Error()))
		return false
	}
	return true
}

func SyncTaskStateLog(ctx context.Context, task *model.Task) {
	_ = syncTaskStateLog(ctx, task)
}

func rollbackTaskUsageStats(task *model.Task, quota int) {
	if task == nil || quota <= 0 {
		return
	}
	model.UpdateUserUsedQuota(task.UserId, -quota)
	model.UpdateChannelUsedQuota(task.ChannelId, -quota)
}

// RefundTaskQuota 统一的任务失败退款逻辑。
// 当异步任务失败时，将预扣的 quota 退还给用户（支持钱包和订阅），并退还令牌额度。
// 返回资金来源是否已成功退还；失败时保留 quota，供显式重试或人工对账。
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) bool {
	quota := task.Quota
	if quota == 0 {
		return true
	}

	if err := taskAdjustFunding(task, -quota); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败 task %s: %s", task.TaskID, err.Error()))
		return false
	}

	taskAdjustTokenQuota(ctx, task, -quota)
	rollbackTaskUsageStats(task, quota)

	if syncTaskStateLog(ctx, task) {
		task.Quota = 0
		if task.ID > 0 {
			if err := task.UpdateQuota(); err != nil {
				logger.LogError(ctx, fmt.Sprintf("退款成功但清除 task quota 失败 task %s: %s", task.TaskID, err.Error()))
			}
		}
		return true
	}

	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["task_status"] = taskLogStatus(task)
	other["reason"] = reason
	other["pre_consumed_quota"] = quota
	other["actual_quota"] = 0
	other["refunded_quota"] = quota
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeError,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     0,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
	})

	// 4. 资金退款完成后再清除持久化标记。
	// 回写失败必须显式告警，避免漏掉潜在的重复退款风险。
	task.Quota = 0
	if task.ID > 0 {
		if err := task.UpdateQuota(); err != nil {
			logger.LogError(ctx, fmt.Sprintf("退款成功但清除 task quota 失败 task %s: %s", task.TaskID, err.Error()))
		}
	}
	return true
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
// clamps 可选：若计算 actualQuota 时发生额度饱和，将其记入日志 admin_info（仅管理员可见）。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	if actualQuota <= 0 {
		return
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota

	if quotaDelta == 0 {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
		task.TaskID,
		logger.LogQuota(quotaDelta),
		logger.LogQuota(actualQuota),
		logger.LogQuota(preConsumedQuota),
		reason,
	))

	// 调整资金来源
	if err := taskAdjustFunding(task, quotaDelta); err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	// 调整令牌额度
	taskAdjustTokenQuota(ctx, task, quotaDelta)

	task.Quota = actualQuota
	if task.ID > 0 {
		if err := task.UpdateQuota(); err != nil {
			logger.LogError(ctx, fmt.Sprintf("差额结算回写 quota 失败 task %s: %s", task.TaskID, err.Error()))
		}
	}

	var logType int
	var logQuota int
	if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
		model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
		model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
	} else {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
		rollbackTaskUsageStats(task, -quotaDelta)
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["task_status"] = taskLogStatus(task)
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	for _, clamp := range clamps {
		attachQuotaSaturationToOther(other, clamp)
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   logType,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     logQuota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
		NodeName:  task.PrivateData.NodeName,
	})
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率和分组倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还。支持钱包和订阅计费来源。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	if totalTokens <= 0 {
		return
	}

	modelName := taskModelName(task)

	// 获取模型价格和倍率
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
	// 不再依赖 hasRatioSetting，避免 AcceptUnsetRatioModel 场景下任务变相免费
	if modelRatio <= 0 {
		return
	}

	// 获取用户和组的倍率信息
	group := task.Group
	if group == "" {
		user, err := model.GetUserById(task.UserId, false)
		if err == nil {
			group = user.Group
		}
	}
	if group == "" {
		return
	}

	groupRatio := ratio_setting.GetGroupRatio(group)
	userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(group, group)

	var finalGroupRatio float64
	if hasUserGroupRatio {
		finalGroupRatio = userGroupRatio
	} else {
		finalGroupRatio = groupRatio
	}

	// 纯 token 差额结算不再叠乘视频分辨率/秒数参数，避免与按次视频单价重复计费。
	actualQuota := int(float64(totalTokens) * modelRatio * finalGroupRatio)

	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f", totalTokens, modelRatio, finalGroupRatio)
	RecalculateTaskQuota(ctx, task, actualQuota, reason)
}
