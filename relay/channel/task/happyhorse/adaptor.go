package happyhorse

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

// ============================
// Request / Response structures (DashScope 格式)
// ============================

// HappyHorseRequest 阿里云百炼 HappyHorse 视频生成请求
type HappyHorseRequest struct {
	Model      string                `json:"model"`
	Input      HappyHorseInput       `json:"input"`
	Parameters *HappyHorseParameters `json:"parameters,omitempty"`
}

// HappyHorseMedia 媒体素材项（reference_image / first_frame / video）
type HappyHorseMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// HappyHorseInput 输入信息
type HappyHorseInput struct {
	Prompt string            `json:"prompt,omitempty"`
	Media  []HappyHorseMedia `json:"media,omitempty"`
}

// HappyHorseParameters 视频参数
type HappyHorseParameters struct {
	Resolution string `json:"resolution,omitempty"` // 480P / 720P / 1080P
	Ratio      string `json:"ratio,omitempty"`      // 宽高比，如 16:9（i2v 不支持）
	Duration   *int   `json:"duration,omitempty"`   // 视频时长（秒）
}

// HappyHorseResponse DashScope 响应
type HappyHorseResponse struct {
	Output    HappyHorseOutput `json:"output"`
	RequestID string           `json:"request_id"`
	Code      string           `json:"code,omitempty"`
	Message   string           `json:"message,omitempty"`
	Usage     *HappyHorseUsage `json:"usage,omitempty"`
}

// HappyHorseOutput 输出信息
type HappyHorseOutput struct {
	TaskID        string `json:"task_id"`
	TaskStatus    string `json:"task_status"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// HappyHorseUsage 使用统计
type HappyHorseUsage struct {
	InputVideoDuration  dto.IntValue `json:"input_video_duration,omitempty"`
	OutputVideoDuration dto.IntValue `json:"output_video_duration,omitempty"`
	Duration            dto.IntValue `json:"duration,omitempty"`
	SR                  dto.IntValue `json:"SR,omitempty"`
	VideoCount          dto.IntValue `json:"video_count,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// ValidateMultipartDirect 负责解析并将原始 TaskSubmitReq 存入 context
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

// BuildRequestHeader 设置 DashScope 异步任务所需请求头
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable") // HTTP 调用只支持异步，必须设置
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_task_request_failed")
	}

	hhReq, err := a.convertToHappyHorseRequest(info, taskReq)
	if err != nil {
		return nil, errors.Wrap(err, "convert_to_happyhorse_request_failed")
	}
	logger.LogJson(c, "happyhorse video request body", hhReq)

	bodyBytes, err := common.Marshal(hhReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_happyhorse_request_failed")
	}
	return bytes.NewReader(bodyBytes), nil
}

func isVideoEditModel(model string) bool {
	return strings.Contains(model, "video-edit")
}

func isImageToVideoModel(model string) bool {
	// 图生视频（i2v）宽高比跟随首帧图像，不支持 ratio 参数
	return strings.Contains(model, "i2v")
}

// convertToHappyHorseRequest 将统一 TaskSubmitReq 转换为 DashScope HappyHorse 请求。
// 素材（media 数组）通过 metadata.media 按 DashScope 原格式透传，
// 适配器仅负责组装 parameters 与默认值。
func (a *TaskAdaptor) convertToHappyHorseRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*HappyHorseRequest, error) {
	upstreamModel := req.Model
	if info.IsModelMapped {
		upstreamModel = info.UpstreamModelName
	}

	hhReq := &HappyHorseRequest{
		Model: upstreamModel,
		Input: HappyHorseInput{
			Prompt: req.Prompt,
		},
		Parameters: &HappyHorseParameters{},
	}

	// 分辨率：优先取请求中的 size（视为分辨率档位，如 720P），否则默认 720P
	if req.Size != "" {
		resolution := strings.ToUpper(req.Size)
		if !strings.HasSuffix(resolution, "P") {
			resolution = resolution + "P"
		}
		hhReq.Parameters.Resolution = resolution
	} else {
		hhReq.Parameters.Resolution = "720P"
	}

	// 时长：video-edit 不接受 duration，由源视频决定
	if !isVideoEditModel(upstreamModel) {
		duration := 5 // 默认 5 秒
		if req.Duration > 0 {
			duration = req.Duration
		} else if req.Seconds != "" {
			if seconds, err := strconv.Atoi(req.Seconds); err == nil {
				duration = seconds
			}
		}
		hhReq.Parameters.Duration = &duration

		// 宽高比：i2v 跟随首帧图像不支持 ratio，其余默认 16:9
		if !isImageToVideoModel(upstreamModel) {
			hhReq.Parameters.Ratio = "16:9"
		}
	}

	// 从 metadata 透传 input.media 与覆盖 parameters。
	// metadata 结构与 DashScope 请求体对齐：{ "media": [...], "parameters": {...} }
	if req.Metadata != nil {
		metadataBytes, err := common.Marshal(req.Metadata)
		if err != nil {
			return nil, errors.Wrap(err, "marshal metadata failed")
		}
		var meta struct {
			Media      []HappyHorseMedia     `json:"media"`
			Parameters *HappyHorseParameters `json:"parameters"`
		}
		if err := common.Unmarshal(metadataBytes, &meta); err != nil {
			return nil, errors.Wrap(err, "unmarshal metadata failed")
		}
		if len(meta.Media) > 0 {
			hhReq.Input.Media = meta.Media
		}
		if meta.Parameters != nil {
			if meta.Parameters.Resolution != "" {
				hhReq.Parameters.Resolution = meta.Parameters.Resolution
			}
			if meta.Parameters.Ratio != "" {
				hhReq.Parameters.Ratio = meta.Parameters.Ratio
			}
			if meta.Parameters.Duration != nil {
				hhReq.Parameters.Duration = meta.Parameters.Duration
			}
		}
	}

	// video-edit 不支持 ratio / duration，确保不下发
	if isVideoEditModel(upstreamModel) {
		hhReq.Parameters.Ratio = ""
		hhReq.Parameters.Duration = nil
	}
	if isImageToVideoModel(upstreamModel) {
		hhReq.Parameters.Ratio = ""
	}

	return hhReq, nil
}

// EstimateBilling 根据用户请求参数计算 OtherRatios（时长、分辨率）。
// 倍率数值由后台模型定价设置配置，此处只负责生成计费键。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	hhReq, err := a.convertToHappyHorseRequest(info, taskReq)
	if err != nil {
		return nil
	}

	otherRatios := make(map[string]float64)
	if hhReq.Parameters.Duration != nil {
		otherRatios["seconds"] = float64(*hhReq.Parameters.Duration)
	}
	if hhReq.Parameters.Resolution != "" {
		otherRatios[fmt.Sprintf("resolution-%s", hhReq.Parameters.Resolution)] = 1
	}
	return otherRatios
}

// DoRequest 委托公共助手发起请求
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 处理上游创建任务的响应
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var hhResp HappyHorseResponse
	if err := common.Unmarshal(responseBody, &hhResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 检查错误
	if hhResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", hhResp.Code, hhResp.Message), "happyhorse_api_error", resp.StatusCode)
		return
	}

	if hhResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// DashScope 原生调用：返回 dashscope 原生响应，仅把 output.task_id 改写为本平台公开 task ID
	if c.GetBool("dashscope_native") {
		nativeResp := responseBody
		if rewritten, e := sjson.SetBytes(nativeResp, "output.task_id", info.PublicTaskID); e == nil {
			nativeResp = rewritten
		}
		c.Data(http.StatusOK, "application/json", nativeResp)
		return hhResp.Output.TaskID, responseBody, nil
	}

	// 转换为 OpenAI 格式响应返回客户端
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = info.PublicTaskID
	openAIResp.TaskID = info.PublicTaskID
	openAIResp.Model = c.GetString("model")
	if openAIResp.Model == "" && info != nil {
		openAIResp.Model = info.OriginModelName
	}
	openAIResp.Status = convertHappyHorseStatus(hhResp.Output.TaskStatus)
	openAIResp.CreatedAt = common.GetTimestamp()

	c.JSON(http.StatusOK, openAIResp)

	return hhResp.Output.TaskID, responseBody, nil
}

// FetchTask 查询任务状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v1/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ParseTaskResult 解析轮询任务结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var hhResp HappyHorseResponse
	if err := common.Unmarshal(respBody, &hhResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch hhResp.Output.TaskStatus {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		// 阿里云百炼直接返回视频 URL，无需额外代理端点
		taskResult.Url = hhResp.Output.VideoURL
	case "FAILED", "CANCELED", "UNKNOWN":
		taskResult.Status = model.TaskStatusFailure
		if hhResp.Message != "" {
			taskResult.Reason = hhResp.Message
		} else if hhResp.Output.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s , message: %s", hhResp.Output.Code, hhResp.Output.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusQueued
	}

	return &taskResult, nil
}

// ConvertToOpenAIVideo 将存储的任务数据转换为 OpenAI 视频格式
func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var hhResp HappyHorseResponse
	if err := common.Unmarshal(task.Data, &hhResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal happyhorse response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Status = convertHappyHorseStatus(hhResp.Output.TaskStatus)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	openAIResp.SetMetadata("url", hhResp.Output.VideoURL)

	if hhResp.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    hhResp.Code,
			Message: hhResp.Message,
		}
	} else if hhResp.Output.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    hhResp.Output.Code,
			Message: hhResp.Output.Message,
		}
	}

	return common.Marshal(openAIResp)
}

// ConvertToDashScopeNative 将存储的任务数据转换为 DashScope 原生响应格式。
// task.Data 由轮询持续刷新为最新的 dashscope 响应，此处仅把 output.task_id
// 改写为本平台公开 task ID，保持其余原生字段（task_status / video_url 等）不变。
func (a *TaskAdaptor) ConvertToDashScopeNative(task *model.Task) ([]byte, error) {
	data := task.Data
	if len(data) == 0 {
		return nil, errors.New("task data is empty")
	}
	rewritten, err := sjson.SetBytes(data, "output.task_id", task.TaskID)
	if err != nil {
		return nil, errors.Wrap(err, "set output.task_id failed")
	}
	return rewritten, nil
}

func convertHappyHorseStatus(status string) string {
	switch status {
	case "PENDING":
		return dto.VideoStatusQueued
	case "RUNNING":
		return dto.VideoStatusInProgress
	case "SUCCEEDED":
		return dto.VideoStatusCompleted
	case "FAILED", "CANCELED", "UNKNOWN":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}
