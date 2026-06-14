package happyhorse

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

type HappyHorseRequest struct {
	Model      string                `json:"model"`
	Input      HappyHorseInput       `json:"input"`
	Parameters *HappyHorseParameters `json:"parameters,omitempty"`
}

type HappyHorseMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type HappyHorseInput struct {
	Prompt string            `json:"prompt,omitempty"`
	Media  []HappyHorseMedia `json:"media,omitempty"`
}

type HappyHorseParameters struct {
	Resolution   string `json:"resolution,omitempty"`
	Ratio        string `json:"ratio,omitempty"`
	Duration     *int   `json:"duration,omitempty"`
	Watermark    *bool  `json:"watermark,omitempty"`
	AudioSetting string `json:"audio_setting,omitempty"`
	Seed         *int   `json:"seed,omitempty"`
}

type HappyHorseResponse struct {
	Output    HappyHorseOutput `json:"output"`
	RequestID string           `json:"request_id"`
	Code      string           `json:"code,omitempty"`
	Message   string           `json:"message,omitempty"`
	Usage     *HappyHorseUsage `json:"usage,omitempty"`
}

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

type HappyHorseUsage struct {
	InputVideoDuration  happyHorseNumber `json:"input_video_duration,omitempty"`
	OutputVideoDuration happyHorseNumber `json:"output_video_duration,omitempty"`
	Duration            happyHorseNumber `json:"duration,omitempty"`
	SR                  happyHorseNumber `json:"SR,omitempty"`
	VideoCount          happyHorseNumber `json:"video_count,omitempty"`
}

type happyHorseRequestMetadata struct {
	Media      []HappyHorseMedia     `json:"media"`
	Parameters *HappyHorseParameters `json:"parameters"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

type happyHorseNumber float64

func (n *happyHorseNumber) UnmarshalJSON(data []byte) error {
	var num float64
	if err := common.Unmarshal(data, &num); err == nil {
		*n = happyHorseNumber(num)
		return nil
	}

	var str string
	if err := common.Unmarshal(data, &str); err != nil {
		return err
	}
	parsed, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return err
	}
	*n = happyHorseNumber(parsed)
	return nil
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_json", http.StatusBadRequest)
	}

	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}

	meta, err := parseHappyHorseMetadata(req)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if err := validateHappyHorseTaskRequest(req.Model, req.Prompt, meta.Media); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	action := constant.TaskActionTextGenerate
	if len(meta.Media) > 0 || req.HasImage() {
		action = constant.TaskActionGenerate
	}

	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	info.Action = action
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")
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
	return strings.Contains(model, "i2v")
}

func isReferenceToVideoModel(model string) bool {
	return strings.Contains(model, "r2v")
}

func parseHappyHorseMetadata(req relaycommon.TaskSubmitReq) (*happyHorseRequestMetadata, error) {
	meta := &happyHorseRequestMetadata{}
	if req.Metadata == nil {
		return meta, nil
	}
	if err := req.UnmarshalMetadata(meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func happyHorseResolutionRatio(resolution string) float64 {
	multiplier, _, ok := ratio_setting.GetVideoResolutionMultiplier("happyhorse-1.0-t2v", resolution)
	if !ok {
		return 1
	}
	return multiplier
}

func happyHorseResolutionLabelFromSR(sr happyHorseNumber) string {
	return ratio_setting.NormalizeVideoResolution(strconv.FormatFloat(float64(sr), 'f', -1, 64))
}

func promptRequiredForModel(model string) bool {
	return !isImageToVideoModel(model)
}

func validateHappyHorseTaskRequest(model, prompt string, media []HappyHorseMedia) error {
	if promptRequiredForModel(model) && strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt is required")
	}

	switch {
	case isImageToVideoModel(model):
		if len(media) != 1 {
			return fmt.Errorf("happyhorse i2v requires exactly one first_frame media")
		}
		if media[0].Type != "first_frame" {
			return fmt.Errorf("happyhorse i2v media type must be first_frame")
		}
	case isReferenceToVideoModel(model):
		if len(media) == 0 {
			return fmt.Errorf("happyhorse r2v requires at least one reference_image")
		}
		for _, item := range media {
			if item.Type != "reference_image" {
				return fmt.Errorf("happyhorse r2v media type must be reference_image")
			}
		}
	case isVideoEditModel(model):
		if len(media) == 0 {
			return fmt.Errorf("happyhorse video-edit requires input media")
		}
		videoCount := 0
		for _, item := range media {
			switch item.Type {
			case "video":
				videoCount++
			case "reference_image":
			default:
				return fmt.Errorf("happyhorse video-edit only supports video and reference_image media")
			}
		}
		if videoCount != 1 {
			return fmt.Errorf("happyhorse video-edit requires exactly one video media")
		}
	}

	return nil
}

func (a *TaskAdaptor) convertToHappyHorseRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*HappyHorseRequest, error) {
	upstreamModel := req.Model
	if info != nil && info.ChannelMeta != nil && info.IsModelMapped {
		upstreamModel = info.UpstreamModelName
	}

	hhReq := &HappyHorseRequest{
		Model: upstreamModel,
		Input: HappyHorseInput{
			Prompt: req.Prompt,
		},
		Parameters: &HappyHorseParameters{
			Resolution: "1080P",
		},
	}

	if req.Size != "" {
		resolution := strings.ToUpper(req.Size)
		if !strings.HasSuffix(resolution, "P") {
			resolution += "P"
		}
		hhReq.Parameters.Resolution = resolution
	}

	if !isVideoEditModel(upstreamModel) {
		duration := 5
		if req.Duration > 0 {
			duration = req.Duration
		} else if req.Seconds != "" {
			if seconds, err := strconv.Atoi(req.Seconds); err == nil {
				duration = seconds
			}
		}
		hhReq.Parameters.Duration = &duration

		if !isImageToVideoModel(upstreamModel) {
			hhReq.Parameters.Ratio = "16:9"
		}
	}

	meta, err := parseHappyHorseMetadata(req)
	if err != nil {
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
		if meta.Parameters.Watermark != nil {
			hhReq.Parameters.Watermark = meta.Parameters.Watermark
		}
		if meta.Parameters.AudioSetting != "" {
			hhReq.Parameters.AudioSetting = meta.Parameters.AudioSetting
		}
		if meta.Parameters.Seed != nil {
			hhReq.Parameters.Seed = meta.Parameters.Seed
		}
	}

	if isVideoEditModel(upstreamModel) {
		hhReq.Parameters.Ratio = ""
		hhReq.Parameters.Duration = nil
	}
	if isImageToVideoModel(upstreamModel) {
		hhReq.Parameters.Ratio = ""
	}

	return hhReq, nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	hhReq, err := a.convertToHappyHorseRequest(info, taskReq)
	if err != nil {
		return nil
	}

	if hhReq.Parameters.Duration == nil {
		return nil
	}
	modelName := taskReq.Model
	if info != nil && info.OriginModelName != "" {
		modelName = info.OriginModelName
	}
	return taskcommon.EstimateVideoOtherRatios(modelName, *hhReq.Parameters.Duration, hhReq.Parameters.Resolution)
}

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	var hhResp HappyHorseResponse
	if err := common.Unmarshal(task.Data, &hhResp); err != nil || hhResp.Usage == nil {
		return 0
	}

	duration := float64(hhResp.Usage.Duration)
	if duration <= 0 {
		duration = float64(hhResp.Usage.OutputVideoDuration)
	}
	if duration <= 0 {
		return 0
	}

	resolution := happyHorseResolutionLabelFromSR(hhResp.Usage.SR)
	return taskcommon.CalculateVideoTaskQuota(task, duration, resolution)
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

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

	if hhResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", hhResp.Code, hhResp.Message), "happyhorse_api_error", resp.StatusCode)
		return
	}
	if hhResp.RequestID != "" {
		c.Set(common.UpstreamRequestIdKey, hhResp.RequestID)
		c.Header(common.UpstreamRequestIdKey, hhResp.RequestID)
	}

	if hhResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	if c.GetBool("dashscope_native") {
		nativeResp := responseBody
		if rewritten, e := sjson.SetBytes(nativeResp, "output.task_id", info.PublicTaskID); e == nil {
			nativeResp = rewritten
		}
		if requestID := c.GetString(common.RequestIdKey); requestID != "" {
			if rewritten, e := sjson.SetBytes(nativeResp, "request_id", requestID); e == nil {
				nativeResp = rewritten
			}
		}
		if hhResp.RequestID != "" {
			if rewritten, e := sjson.SetBytes(nativeResp, "upstream_request_id", hhResp.RequestID); e == nil {
				nativeResp = rewritten
			}
		}
		c.Data(http.StatusOK, "application/json", nativeResp)
		return hhResp.Output.TaskID, responseBody, nil
	}

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

func (a *TaskAdaptor) ConvertToDashScopeNative(task *model.Task) ([]byte, error) {
	data := task.Data
	if len(data) == 0 {
		return nil, errors.New("task data is empty")
	}
	rewritten, err := sjson.SetBytes(data, "output.task_id", task.TaskID)
	if err != nil {
		return nil, errors.Wrap(err, "set output.task_id failed")
	}
	if task.PrivateData.RequestID != "" {
		if updated, setErr := sjson.SetBytes(rewritten, "request_id", task.PrivateData.RequestID); setErr == nil {
			rewritten = updated
		}
	}
	if task.PrivateData.UpstreamRequestID != "" {
		if updated, setErr := sjson.SetBytes(rewritten, "upstream_request_id", task.PrivateData.UpstreamRequestID); setErr == nil {
			rewritten = updated
		}
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
