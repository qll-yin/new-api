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
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

const (
	dashScopeVideoSynthesisPath  = "/api/v1/services/aigc/video-generation/video-synthesis"
	dashScopeImage2VideoTaskPath = "/api/v1/services/aigc/image2video/video-synthesis"
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

type LegacyWanAnimateRequest struct {
	Model      string                      `json:"model"`
	Input      LegacyWanAnimateInput       `json:"input"`
	Parameters *LegacyWanAnimateParameters `json:"parameters,omitempty"`
}

type LegacyWanAnimateInput struct {
	ImageURL  string `json:"image_url,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	Watermark *bool  `json:"watermark,omitempty"`
}

type LegacyWanAnimateParameters struct {
	Mode string `json:"mode,omitempty"`
}

type HappyHorseResponse struct {
	Output    HappyHorseOutput `json:"output"`
	RequestID string           `json:"request_id"`
	Code      string           `json:"code,omitempty"`
	Message   string           `json:"message,omitempty"`
	Usage     *HappyHorseUsage `json:"usage,omitempty"`
}

type HappyHorseOutput struct {
	TaskID        string                     `json:"task_id"`
	TaskStatus    string                     `json:"task_status"`
	SubmitTime    string                     `json:"submit_time,omitempty"`
	ScheduledTime string                     `json:"scheduled_time,omitempty"`
	EndTime       string                     `json:"end_time,omitempty"`
	OrigPrompt    string                     `json:"orig_prompt,omitempty"`
	VideoURL      string                     `json:"video_url,omitempty"`
	Results       *LegacyWanAnimateResults   `json:"results,omitempty"`
	Code          string                     `json:"code,omitempty"`
	Message       string                     `json:"message,omitempty"`
}

type LegacyWanAnimateResults struct {
	VideoURL string `json:"video_url,omitempty"`
}

type HappyHorseUsage struct {
	InputVideoDuration  happyHorseNumber `json:"input_video_duration,omitempty"`
	OutputVideoDuration happyHorseNumber `json:"output_video_duration,omitempty"`
	Duration            happyHorseNumber `json:"duration,omitempty"`
	SR                  happyHorseNumber `json:"SR,omitempty"`
	VideoCount          happyHorseNumber `json:"video_count,omitempty"`
	VideoDuration       happyHorseNumber `json:"video_duration,omitempty"`
	VideoRatio          string           `json:"video_ratio,omitempty"`
}

type happyHorseRequestMetadata struct {
	Input      *legacyWanAnimateMetadata `json:"input"`
	Media      []HappyHorseMedia         `json:"media"`
	Parameters map[string]any            `json:"parameters"`
}

type legacyWanAnimateMetadata struct {
	ImageURL  string `json:"image_url"`
	VideoURL  string `json:"video_url"`
	Watermark *bool  `json:"watermark"`
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
	validationModel := resolveValidationModel(c, req.Model)

	meta, err := parseHappyHorseMetadata(req)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if err := validateHappyHorseTaskRequest(validationModel, req.Prompt, meta); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	action := constant.TaskActionTextGenerate
	if len(meta.Media) > 0 || req.HasImage() || meta.Input != nil {
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
	modelName := ""
	if info != nil {
		modelName = info.UpstreamModelName
		if modelName == "" {
			modelName = info.OriginModelName
		}
	}
	if isLegacyWanAnimateModel(modelName) {
		return fmt.Sprintf("%s%s", a.baseURL, dashScopeImage2VideoTaskPath), nil
	}
	return fmt.Sprintf("%s%s", a.baseURL, dashScopeVideoSynthesisPath), nil
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

	body, err := a.convertToDashScopeRequest(info, taskReq)
	if err != nil {
		return nil, errors.Wrap(err, "convert_to_dashscope_request_failed")
	}
	logger.LogJson(c, "alibailian video request body", body)

	bodyBytes, err := common.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_dashscope_request_failed")
	}
	return bytes.NewReader(bodyBytes), nil
}

func isLegacyWanAnimateModel(model string) bool {
	return strings.HasPrefix(model, "wan2.2-animate-")
}

func isWan27VideoModel(model string) bool {
	return strings.HasPrefix(model, "wan2.7-")
}

func isVideoEditModel(model string) bool {
	return strings.Contains(model, "video-edit") || strings.Contains(model, "videoedit")
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

func resolveValidationModel(c *gin.Context, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
		},
	}
	if err := relayhelper.ModelMappedHelper(c, info, nil); err != nil {
		return model
	}
	if info.IsModelMapped && strings.TrimSpace(info.UpstreamModelName) != "" {
		return info.UpstreamModelName
	}
	return model
}

func promptRequiredForModel(model string) bool {
	return !isImageToVideoModel(model) && !isLegacyWanAnimateModel(model)
}

func validateHappyHorseTaskRequest(model, prompt string, meta *happyHorseRequestMetadata) error {
	if meta == nil {
		meta = &happyHorseRequestMetadata{}
	}
	if promptRequiredForModel(model) && strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt is required")
	}

	if isLegacyWanAnimateModel(model) {
		if meta.Input == nil {
			return fmt.Errorf("%s requires input", model)
		}
		if strings.TrimSpace(meta.Input.ImageURL) == "" {
			return fmt.Errorf("%s requires input.image_url", model)
		}
		if strings.TrimSpace(meta.Input.VideoURL) == "" {
			return fmt.Errorf("%s requires input.video_url", model)
		}
		return nil
	}

	switch {
	case isImageToVideoModel(model):
		if len(meta.Media) == 0 {
			return fmt.Errorf("%s requires first_frame media", model)
		}
		allowedTypes := map[string]bool{
			"first_frame":   true,
			"last_frame":    true,
			"driving_audio": true,
		}
		firstFrameCount := 0
		for _, item := range meta.Media {
			if !allowedTypes[item.Type] {
				return fmt.Errorf("%s media type must be first_frame, last_frame or driving_audio", model)
			}
			if item.Type == "first_frame" {
				firstFrameCount++
			}
		}
		if firstFrameCount != 1 {
			return fmt.Errorf("%s requires exactly one first_frame media", model)
		}
	case isReferenceToVideoModel(model):
		if len(meta.Media) == 0 {
			return fmt.Errorf("%s requires reference media", model)
		}
		allowedTypes := map[string]bool{
			"reference_image": true,
			"reference_video": true,
			"reference_voice": true,
		}
		referenceFound := false
		for _, item := range meta.Media {
			if !allowedTypes[item.Type] {
				return fmt.Errorf("%s media type must be reference_image, reference_video or reference_voice", model)
			}
			if item.Type == "reference_image" || item.Type == "reference_video" {
				referenceFound = true
			}
		}
		if !referenceFound {
			return fmt.Errorf("%s requires reference_image or reference_video media", model)
		}
	case isVideoEditModel(model):
		if len(meta.Media) == 0 {
			return fmt.Errorf("%s requires input media", model)
		}
		videoCount := 0
		for _, item := range meta.Media {
			switch item.Type {
			case "video":
				videoCount++
			case "reference_image":
			default:
				return fmt.Errorf("%s only supports video and reference_image media", model)
			}
		}
		if videoCount != 1 {
			return fmt.Errorf("%s requires exactly one video media", model)
		}
	}

	return nil
}

func getUpstreamModel(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) string {
	upstreamModel := req.Model
	if info != nil && info.ChannelMeta != nil && info.IsModelMapped && info.UpstreamModelName != "" {
		upstreamModel = info.UpstreamModelName
	}
	return upstreamModel
}

func defaultVideoResolutionForModel(model string) string {
	if isLegacyWanAnimateModel(model) {
		return ""
	}
	return "1080P"
}

func parseDuration(req relaycommon.TaskSubmitReq) (int, error) {
	if req.Duration > 0 {
		return req.Duration, nil
	}
	if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil {
			return 0, errors.Wrap(err, "convert seconds to int failed")
		}
		if seconds > 0 {
			return seconds, nil
		}
	}
	return 5, nil
}

func stringFromMap(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	raw, ok := params[key]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func boolPtrFromMap(params map[string]any, key string) *bool {
	if params == nil {
		return nil
	}
	raw, ok := params[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case bool:
		return &v
	case string:
		if parsed, err := strconv.ParseBool(v); err == nil {
			return &parsed
		}
	}
	return nil
}

func intPtrFromMap(params map[string]any, key string) *int {
	if params == nil {
		return nil
	}
	raw, ok := params[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case int:
		return &v
	case int32:
		value := int(v)
		return &value
	case int64:
		value := int(v)
		return &value
	case float64:
		value := int(v)
		return &value
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			return &parsed
		}
	}
	return nil
}

func buildHappyHorseParameters(model string, req relaycommon.TaskSubmitReq, meta *happyHorseRequestMetadata) (*HappyHorseParameters, error) {
	parameters := &HappyHorseParameters{
		Resolution: defaultVideoResolutionForModel(model),
	}
	if req.Size != "" {
		resolution := strings.ToUpper(req.Size)
		if !strings.HasSuffix(resolution, "P") {
			resolution += "P"
		}
		parameters.Resolution = resolution
	}

	if !isVideoEditModel(model) {
		duration, err := parseDuration(req)
		if err != nil {
			return nil, err
		}
		parameters.Duration = &duration
		if !isImageToVideoModel(model) {
			parameters.Ratio = "16:9"
		}
	}

	if meta != nil {
		if resolution := stringFromMap(meta.Parameters, "resolution"); resolution != "" {
			parameters.Resolution = strings.ToUpper(strings.TrimSpace(resolution))
		}
		if ratio := stringFromMap(meta.Parameters, "ratio"); ratio != "" {
			parameters.Ratio = ratio
		}
		if duration := intPtrFromMap(meta.Parameters, "duration"); duration != nil {
			parameters.Duration = duration
		}
		if watermark := boolPtrFromMap(meta.Parameters, "watermark"); watermark != nil {
			parameters.Watermark = watermark
		}
		if audioSetting := stringFromMap(meta.Parameters, "audio_setting"); audioSetting != "" {
			parameters.AudioSetting = audioSetting
		}
		if seed := intPtrFromMap(meta.Parameters, "seed"); seed != nil {
			parameters.Seed = seed
		}
	}

	if isVideoEditModel(model) {
		parameters.Ratio = ""
		parameters.Duration = nil
	}
	if isImageToVideoModel(model) {
		parameters.Ratio = ""
	}

	return parameters, nil
}

func (a *TaskAdaptor) convertToDashScopeRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (any, error) {
	upstreamModel := getUpstreamModel(info, req)
	meta, err := parseHappyHorseMetadata(req)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	if isLegacyWanAnimateModel(upstreamModel) {
		return convertToLegacyWanAnimateRequest(upstreamModel, meta)
	}
	return convertToHappyHorseRequest(upstreamModel, req, meta)
}

func convertToHappyHorseRequest(upstreamModel string, req relaycommon.TaskSubmitReq, meta *happyHorseRequestMetadata) (*HappyHorseRequest, error) {
	parameters, err := buildHappyHorseParameters(upstreamModel, req, meta)
	if err != nil {
		return nil, err
	}

	hhReq := &HappyHorseRequest{
		Model: upstreamModel,
		Input: HappyHorseInput{
			Prompt: req.Prompt,
		},
		Parameters: parameters,
	}
	if meta != nil && len(meta.Media) > 0 {
		hhReq.Input.Media = meta.Media
	}
	return hhReq, nil
}

func convertToLegacyWanAnimateRequest(upstreamModel string, meta *happyHorseRequestMetadata) (*LegacyWanAnimateRequest, error) {
	if meta == nil || meta.Input == nil {
		return nil, fmt.Errorf("%s requires input", upstreamModel)
	}
	request := &LegacyWanAnimateRequest{
		Model: upstreamModel,
		Input: LegacyWanAnimateInput{
			ImageURL:  meta.Input.ImageURL,
			VideoURL:  meta.Input.VideoURL,
			Watermark: meta.Input.Watermark,
		},
	}
	mode := stringFromMap(meta.Parameters, "mode")
	if mode != "" {
		request.Parameters = &LegacyWanAnimateParameters{Mode: mode}
	}
	return request, nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	upstreamModel := getUpstreamModel(info, taskReq)
	if isLegacyWanAnimateModel(upstreamModel) {
		return nil
	}

	body, err := a.convertToDashScopeRequest(info, taskReq)
	if err != nil {
		return nil
	}

	hhReq, ok := body.(*HappyHorseRequest)
	if !ok || hhReq.Parameters == nil || hhReq.Parameters.Duration == nil {
		return nil
	}

	modelName := taskReq.Model
	if info != nil && info.OriginModelName != "" {
		modelName = info.OriginModelName
	}
	return taskcommon.EstimateVideoOtherRatios(modelName, *hhReq.Parameters.Duration, hhReq.Parameters.Resolution)
}

func findSuccessfulVideoURL(resp *HappyHorseResponse) string {
	if resp == nil {
		return ""
	}
	if resp.Output.VideoURL != "" {
		return resp.Output.VideoURL
	}
	if resp.Output.Results != nil {
		return resp.Output.Results.VideoURL
	}
	return ""
}

func findSuccessfulDurationSeconds(resp *HappyHorseResponse) float64 {
	if resp == nil || resp.Usage == nil {
		return 0
	}
	duration := float64(resp.Usage.Duration)
	if duration <= 0 {
		duration = float64(resp.Usage.OutputVideoDuration)
	}
	if duration <= 0 {
		duration = float64(resp.Usage.VideoDuration)
	}
	return duration
}

func findSuccessfulResolution(resp *HappyHorseResponse) string {
	if resp == nil || resp.Usage == nil {
		return ""
	}
	resolution := happyHorseResolutionLabelFromSR(resp.Usage.SR)
	if resolution != "" {
		return resolution
	}
	return ratio_setting.NormalizeVideoResolution(resp.Usage.VideoRatio)
}

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	var hhResp HappyHorseResponse
	if err := common.Unmarshal(task.Data, &hhResp); err != nil || hhResp.Usage == nil {
		return 0
	}

	duration := findSuccessfulDurationSeconds(&hhResp)
	if duration <= 0 {
		return 0
	}

	modelName := ""
	if task != nil && task.PrivateData.BillingContext != nil {
		modelName = task.PrivateData.BillingContext.OriginModelName
	}
	if modelName == "" && task != nil {
		modelName = task.Properties.OriginModelName
	}
	if isLegacyWanAnimateModel(modelName) {
		return 0
	}

	resolution := findSuccessfulResolution(&hhResp)
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
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", hhResp.Code, hhResp.Message), "alibailian_api_error", resp.StatusCode)
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
		taskResult.Url = findSuccessfulVideoURL(&hhResp)
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
		return nil, errors.Wrap(err, "unmarshal alibailian response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Status = convertHappyHorseStatus(hhResp.Output.TaskStatus)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	openAIResp.SetMetadata("url", findSuccessfulVideoURL(&hhResp))

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

func happyHorseResolutionLabelFromSR(sr happyHorseNumber) string {
	return ratio_setting.NormalizeVideoResolution(strconv.FormatFloat(float64(sr), 'f', -1, 64))
}
