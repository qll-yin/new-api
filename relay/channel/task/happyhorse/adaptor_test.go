package happyhorse

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestValidateRequestAndSetActionAcceptsDashScopeNativeBodyOnUnifiedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"happyhorse-1.0-i2v","input":{"prompt":"一只猫在草地上奔跑","media":[{"type":"first_frame","url":"https://example.com/first.png"}]},"parameters":{"resolution":"1080P","duration":5}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected dashscope native body on unified route to pass, got error: %v", taskErr)
	}

	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		t.Fatalf("expected task request to be stored, got error: %v", err)
	}
	if taskReq.Prompt != "一只猫在草地上奔跑" {
		t.Fatalf("expected prompt from input.prompt, got %q", taskReq.Prompt)
	}

	converted, err := adaptor.convertToDashScopeRequest(info, taskReq)
	if err != nil {
		t.Fatalf("expected dashscope request conversion to succeed, got error: %v", err)
	}
	hhReq, ok := converted.(*HappyHorseRequest)
	if !ok {
		t.Fatalf("expected happyhorse request, got %T", converted)
	}
	if hhReq.Input.Prompt != "一只猫在草地上奔跑" {
		t.Fatalf("expected prompt to be preserved, got %q", hhReq.Input.Prompt)
	}
	if len(hhReq.Input.Media) != 1 || hhReq.Input.Media[0].Type != "first_frame" {
		t.Fatalf("expected first_frame media to be preserved, got %+v", hhReq.Input.Media)
	}
	if hhReq.Parameters == nil || hhReq.Parameters.Resolution != "1080P" || hhReq.Parameters.Duration == nil || *hhReq.Parameters.Duration != 5 {
		t.Fatalf("expected parameters to be preserved, got %+v", hhReq.Parameters)
	}
}
func TestValidateRequestAndSetActionAcceptsDashScopeNativeBodyMatrixOnUnifiedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "happyhorse t2v",
			body: `{"model":"happyhorse-1.0-t2v","input":{"prompt":"城市夜景"},"parameters":{"resolution":"720P","ratio":"16:9","duration":5}}`,
		},
		{
			name: "happyhorse i2v",
			body: `{"model":"happyhorse-1.0-i2v","input":{"prompt":"一只猫在草地上奔跑","media":[{"type":"first_frame","url":"https://example.com/first.png"}]},"parameters":{"resolution":"1080P","duration":5}}`,
		},
		{
			name: "happyhorse r2v",
			body: `{"model":"happyhorse-1.0-r2v","input":{"prompt":"参考人物跳舞","media":[{"type":"reference_image","url":"https://example.com/ref.png"}]},"parameters":{"resolution":"720P","ratio":"16:9","duration":5}}`,
		},
		{
			name: "happyhorse video edit",
			body: `{"model":"happyhorse-1.0-video-edit","input":{"prompt":"给视频换衣服","media":[{"type":"video","url":"https://example.com/input.mp4"},{"type":"reference_image","url":"https://example.com/ref.png"}]},"parameters":{"resolution":"1080P"}}`,
		},
		{
			name: "wan27 videoedit",
			body: `{"model":"wan2.7-videoedit","input":{"prompt":"转换为黏土风格","media":[{"type":"video","url":"https://example.com/input.mp4"}]},"parameters":{"resolution":"720P"}}`,
		},
		{
			name: "legacy animate",
			body: `{"model":"wan2.2-animate-mix","input":{"image_url":"https://example.com/image.png","video_url":"https://example.com/video.mp4","watermark":false},"parameters":{"mode":"wan-pro"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req

			adaptor := &TaskAdaptor{}
			info := &relaycommon.RelayInfo{}
			if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
				t.Fatalf("expected dashscope native body on unified route to pass, got error: %v", taskErr)
			}

			taskReq, err := relaycommon.GetTaskRequest(c)
			if err != nil {
				t.Fatalf("expected task request to be stored, got error: %v", err)
			}
			if _, err := adaptor.convertToDashScopeRequest(info, taskReq); err != nil {
				t.Fatalf("expected dashscope request conversion to succeed, got error: %v", err)
			}
		})
	}
}
func TestValidateRequestAndSetActionAllowsWan27I2VWithoutPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"wan2.7-i2v-2026-04-25","metadata":{"media":[{"type":"first_frame","url":"https://example.com/first.png"}],"parameters":{"duration":5}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected wan2.7 i2v request without prompt to pass, got error: %v", taskErr)
	}
	if info.Action == "" {
		t.Fatal("expected action to be set")
	}
}

func TestValidateRequestAndSetActionAllowsLegacyAnimateWithoutPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"wan2.2-animate-move","metadata":{"input":{"image_url":"https://example.com/image.png","video_url":"https://example.com/video.mp4","watermark":false},"parameters":{"mode":"wan-pro"}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected legacy animate request to pass, got error: %v", taskErr)
	}
	if info.Action != constant.TaskActionGenerate {
		t.Fatalf("expected generate action, got %q", info.Action)
	}
}

func TestValidateRequestAndSetActionUsesMappedWan27I2VModelForValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"custom-wan-i2v","metadata":{"media":[{"type":"first_frame","url":"https://example.com/first.png"}],"parameters":{"duration":5}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	c.Set("model_mapping", `{"custom-wan-i2v":"wan2.7-i2v-2026-04-25"}`)

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected mapped wan2.7 i2v request without prompt to pass, got error: %v", taskErr)
	}
	if info.Action != constant.TaskActionGenerate {
		t.Fatalf("expected generate action, got %q", info.Action)
	}
}

func TestValidateRequestAndSetActionUsesMappedLegacyWanModelForValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"custom-wan-animate","metadata":{"input":{"image_url":"https://example.com/image.png","video_url":"https://example.com/video.mp4","watermark":false},"parameters":{"mode":"wan-pro"}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	c.Set("model_mapping", `{"custom-wan-animate":"wan2.2-animate-move"}`)

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected mapped legacy wan request without prompt to pass, got error: %v", taskErr)
	}
	if info.Action != constant.TaskActionGenerate {
		t.Fatalf("expected generate action, got %q", info.Action)
	}
}

func TestBuildRequestURLUsesLegacyWanEndpoint(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "wan2.2-animate-move",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://dashscope.example.com",
		},
	}
	adaptor.Init(info)

	url, err := adaptor.BuildRequestURL(info)
	if err != nil {
		t.Fatalf("expected build request url to succeed, got error: %v", err)
	}
	if want := "https://dashscope.example.com/api/v1/services/aigc/image2video/video-synthesis"; url != want {
		t.Fatalf("expected legacy animate url %q, got %q", want, url)
	}
}

func TestConvertToHappyHorseRequestSupportsWan27ReferenceVideo(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	watermark := false
	seed := 7

	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-r2v",
		Prompt: "reference video animation",
		Metadata: map[string]interface{}{
			"media": []map[string]interface{}{
				{
					"type": "reference_video",
					"url":  "https://example.com/reference.mp4",
				},
				{
					"type": "reference_voice",
					"url":  "https://example.com/reference.wav",
				},
			},
			"parameters": map[string]interface{}{
				"resolution":    "720P",
				"ratio":         "16:9",
				"duration":      8,
				"watermark":     watermark,
				"audio_setting": "origin",
				"seed":          seed,
			},
		},
	}

	body, err := adaptor.convertToDashScopeRequest(info, req)
	if err != nil {
		t.Fatalf("expected request conversion to succeed, got error: %v", err)
	}
	hhReq, ok := body.(*HappyHorseRequest)
	if !ok {
		t.Fatalf("expected happyhorse request body, got %T", body)
	}
	if hhReq.Parameters.Resolution != "720P" {
		t.Fatalf("expected resolution 720P, got %q", hhReq.Parameters.Resolution)
	}
	if hhReq.Parameters.Duration == nil || *hhReq.Parameters.Duration != 8 {
		t.Fatalf("expected duration 8 to be preserved, got %+v", hhReq.Parameters.Duration)
	}
	if hhReq.Parameters.Ratio != "16:9" {
		t.Fatalf("expected ratio 16:9, got %q", hhReq.Parameters.Ratio)
	}
	if hhReq.Parameters.Watermark == nil || *hhReq.Parameters.Watermark != watermark {
		t.Fatalf("expected watermark=%v to be preserved", watermark)
	}
	if hhReq.Parameters.AudioSetting != "origin" {
		t.Fatalf("expected audio_setting to be preserved, got %q", hhReq.Parameters.AudioSetting)
	}
	if hhReq.Parameters.Seed == nil || *hhReq.Parameters.Seed != seed {
		t.Fatalf("expected seed=%d to be preserved", seed)
	}
	if len(hhReq.Input.Media) != 2 {
		t.Fatalf("expected 2 media items, got %d", len(hhReq.Input.Media))
	}
}

func TestConvertToHappyHorseRequestOmitsVideoEditDurationAndRatio(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-videoedit",
		Prompt: "edit this video",
		Metadata: map[string]interface{}{
			"media": []map[string]interface{}{
				{
					"type": "video",
					"url":  "https://example.com/video.mp4",
				},
			},
		},
	}

	body, err := adaptor.convertToDashScopeRequest(info, req)
	if err != nil {
		t.Fatalf("expected request conversion to succeed, got error: %v", err)
	}
	hhReq := body.(*HappyHorseRequest)
	if hhReq.Parameters.Duration != nil {
		t.Fatal("expected videoedit duration to be omitted")
	}
	if hhReq.Parameters.Ratio != "" {
		t.Fatalf("expected videoedit ratio to be omitted, got %q", hhReq.Parameters.Ratio)
	}
}

func TestConvertToLegacyWanAnimateRequestPreservesInput(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}

	req := relaycommon.TaskSubmitReq{
		Model: "wan2.2-animate-mix",
		Metadata: map[string]interface{}{
			"input": map[string]interface{}{
				"image_url": "https://example.com/image.png",
				"video_url": "https://example.com/video.mp4",
				"watermark": false,
			},
			"parameters": map[string]interface{}{
				"mode": "wan-pro",
			},
		},
	}

	body, err := adaptor.convertToDashScopeRequest(info, req)
	if err != nil {
		t.Fatalf("expected legacy request conversion to succeed, got error: %v", err)
	}
	legacyReq, ok := body.(*LegacyWanAnimateRequest)
	if !ok {
		t.Fatalf("expected legacy animate request body, got %T", body)
	}
	if legacyReq.Input.ImageURL != "https://example.com/image.png" {
		t.Fatalf("expected image_url to be preserved, got %q", legacyReq.Input.ImageURL)
	}
	if legacyReq.Input.VideoURL != "https://example.com/video.mp4" {
		t.Fatalf("expected video_url to be preserved, got %q", legacyReq.Input.VideoURL)
	}
	if legacyReq.Parameters == nil || legacyReq.Parameters.Mode != "wan-pro" {
		t.Fatalf("expected mode wan-pro, got %+v", legacyReq.Parameters)
	}
}

func TestEstimateBillingUsesWan27ResolutionMultiplier(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"wan2.7-t2v","prompt":"city lights","size":"1080P","duration":5}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected task request to be valid, got %v", taskErr)
	}

	ratios := adaptor.EstimateBilling(c, info)
	if got := ratios["seconds"]; got != 5 {
		t.Fatalf("expected seconds ratio 5, got %v", got)
	}
	wantResolutionRatio := 0.24 / 0.14
	if got := ratios["resolution-1080P"]; got != wantResolutionRatio {
		t.Fatalf("expected resolution ratio %v, got %v", wantResolutionRatio, got)
	}
}

func TestEstimateBillingReturnsNilForLegacyAnimate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"wan2.2-animate-move","metadata":{"input":{"image_url":"https://example.com/image.png","video_url":"https://example.com/video.mp4"}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected task request to be valid, got %v", taskErr)
	}

	if ratios := adaptor.EstimateBilling(c, info); ratios != nil {
		t.Fatalf("expected legacy animate estimate billing to be nil, got %+v", ratios)
	}
}

func TestAdjustBillingOnCompleteUsesActualUsageAndGroupRatio(t *testing.T) {
	respBody := []byte(`{
		"output": {
			"task_id": "upstream-task",
			"task_status": "SUCCEEDED",
			"video_url": "https://example.com/video.mp4"
		},
		"usage": {
			"duration": 5,
			"SR": 1080
		}
	}`)

	task := &model.Task{
		Properties: model.Properties{
			OriginModelName: "wan2.7-t2v",
		},
		Data: respBody,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "wan2.7-t2v",
				ModelPrice:      0.14,
				GroupRatio:      1.5,
				OtherRatios: map[string]float64{
					"seconds":          5,
					"resolution-1080P": 0.24 / 0.14,
				},
			},
		},
	}

	adaptor := &TaskAdaptor{}
	got := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	want := 900000
	if got != want {
		t.Fatalf("expected actual quota %d, got %d", want, got)
	}
}

func TestAdjustBillingOnCompleteVideoEditUsesUsageDurationAndSR(t *testing.T) {
	respBody := []byte(`{
		"output": {
			"task_id": "upstream-task",
			"task_status": "SUCCEEDED",
			"video_url": "https://example.com/video.mp4"
		},
		"usage": {
			"duration": 10,
			"SR": 1080
		}
	}`)

	task := &model.Task{
		Properties: model.Properties{
			OriginModelName: "happyhorse-1.0-video-edit",
		},
		Data: respBody,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "happyhorse-1.0-video-edit",
				ModelPrice:      0.14,
				GroupRatio:      1,
				PerCallBilling:  true,
			},
		},
	}

	adaptor := &TaskAdaptor{}
	got := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
	want := 1200000 // $0.14 * 10s * (0.24 / 0.14) * 500000 quota/USD
	if got != want {
		t.Fatalf("expected video-edit 1080P quota %d, got %d", want, got)
	}
}
func TestAdjustBillingOnCompleteAllBailianVideoModelsUseUsage(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		usageJSON string
		wantQuota int
	}{
		{
			name:      "happyhorse t2v 1080p",
			model:     "happyhorse-1.0-t2v",
			usageJSON: `"duration": 10, "SR": 1080`,
			wantQuota: 1200000,
		},
		{
			name:      "happyhorse i2v 1080p",
			model:     "happyhorse-1.0-i2v",
			usageJSON: `"duration": 10, "SR": 1080`,
			wantQuota: 1200000,
		},
		{
			name:      "happyhorse r2v 1080p",
			model:     "happyhorse-1.0-r2v",
			usageJSON: `"duration": 10, "SR": 1080`,
			wantQuota: 1200000,
		},
		{
			name:      "wan27 t2v 1080p",
			model:     "wan2.7-t2v",
			usageJSON: `"duration": 10, "SR": 1080`,
			wantQuota: 1200000,
		},
		{
			name:      "wan27 i2v 1080p",
			model:     "wan2.7-i2v-2026-04-25",
			usageJSON: `"duration": 10, "SR": 1080`,
			wantQuota: 1200000,
		},
		{
			name:      "wan27 r2v 1080p",
			model:     "wan2.7-r2v",
			usageJSON: `"duration": 10, "SR": 1080`,
			wantQuota: 1200000,
		},
		{
			name:      "wan27 videoedit 1080p",
			model:     "wan2.7-videoedit",
			usageJSON: `"duration": 10, "SR": 1080`,
			wantQuota: 1200000,
		},
		{
			name:      "legacy animate move duration",
			model:     "wan2.2-animate-move",
			usageJSON: `"video_duration": 5.2, "video_ratio": "standard"`,
			wantQuota: 364000,
		},
		{
			name:      "legacy animate mix duration",
			model:     "wan2.2-animate-mix",
			usageJSON: `"video_duration": 5.2, "video_ratio": "pro"`,
			wantQuota: 364000,
		},
	}

	adaptor := &TaskAdaptor{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBody := []byte(`{
				"output": {
					"task_id": "upstream-task",
					"task_status": "SUCCEEDED",
					"video_url": "https://example.com/video.mp4"
				},
				"usage": {` + tt.usageJSON + `}
			}`)

			task := &model.Task{
				Properties: model.Properties{
					OriginModelName: tt.model,
				},
				Data: respBody,
				PrivateData: model.TaskPrivateData{
					BillingContext: &model.TaskBillingContext{
						OriginModelName: tt.model,
						ModelPrice:      0.14,
						GroupRatio:      1,
						PerCallBilling:  true,
					},
				},
			}

			got := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
			if got != tt.wantQuota {
				t.Fatalf("expected quota %d, got %d", tt.wantQuota, got)
			}
		})
	}
}
func TestParseTaskResultSupportsLegacyAnimateResponse(t *testing.T) {
	body := []byte(`{
		"output": {
			"task_id": "upstream-task",
			"task_status": "SUCCEEDED",
			"results": {
				"video_url": "https://example.com/video.mp4"
			}
		},
		"usage": {
			"video_duration": 6.62,
			"video_ratio": "720P"
		}
	}`)

	adaptor := &TaskAdaptor{}
	taskInfo, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("expected legacy animate response to parse, got error: %v", err)
	}
	if taskInfo.Status != model.TaskStatusSuccess {
		t.Fatalf("expected success status, got %s", taskInfo.Status)
	}
	if taskInfo.Url != "https://example.com/video.mp4" {
		t.Fatalf("expected video url to be read from output.results.video_url, got %q", taskInfo.Url)
	}
}

func TestConvertToOpenAIVideoSupportsLegacyAnimateResponse(t *testing.T) {
	task := &model.Task{
		TaskID: "task_123",
		Properties: model.Properties{
			OriginModelName: "wan2.2-animate-move",
		},
		Data: []byte(`{
			"output": {
				"task_id": "upstream-task",
				"task_status": "SUCCEEDED",
				"results": {
					"video_url": "https://example.com/video.mp4"
				}
			}
		}`),
	}

	adaptor := &TaskAdaptor{}
	data, err := adaptor.ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("expected convert to openai video to succeed, got error: %v", err)
	}
	if !strings.Contains(string(data), "https://example.com/video.mp4") {
		t.Fatalf("expected openai video response to contain legacy video url, got %s", string(data))
	}
}

func TestConvertToDashScopeNativeRewritesTaskID(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Data: []byte(`{
			"output": {
				"task_id": "task_upstream",
				"task_status": "SUCCEEDED"
			},
			"request_id": "request_upstream"
		}`),
		PrivateData: model.TaskPrivateData{
			RequestID:         "request_local",
			UpstreamRequestID: "request_upstream",
		},
	}

	adaptor := &TaskAdaptor{}
	data, err := adaptor.ConvertToDashScopeNative(task)
	if err != nil {
		t.Fatalf("expected dashscope native conversion to succeed, got error: %v", err)
	}
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("expected dashscope native response to stay json, got error: %v", err)
	}
	output := payload["output"].(map[string]any)
	if output["task_id"] != "task_public" {
		t.Fatalf("expected public task id, got %v", output["task_id"])
	}
}
