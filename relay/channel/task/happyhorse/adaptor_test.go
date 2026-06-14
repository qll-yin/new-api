package happyhorse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestValidateRequestAndSetActionAllowsI2VWithoutPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"happyhorse-1.0-i2v","metadata":{"media":[{"type":"first_frame","url":"https://example.com/first.png"}],"parameters":{"duration":5}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected i2v request without prompt to pass, got error: %v", taskErr)
	}
	if info.Action == "" {
		t.Fatal("expected action to be set")
	}
}

func TestConvertToHappyHorseRequestUsesOfficialDefaultsAndParameters(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	watermark := false
	seed := 7

	req := relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.0-video-edit",
		Prompt: "edit this video",
		Metadata: map[string]interface{}{
			"media": []map[string]interface{}{
				{
					"type": "video",
					"url":  "https://example.com/video.mp4",
				},
			},
			"parameters": map[string]interface{}{
				"watermark":     watermark,
				"audio_setting": "origin",
				"seed":          seed,
			},
		},
	}

	hhReq, err := adaptor.convertToHappyHorseRequest(info, req)
	if err != nil {
		t.Fatalf("expected request conversion to succeed, got error: %v", err)
	}
	if hhReq.Parameters.Resolution != "1080P" {
		t.Fatalf("expected default resolution 1080P, got %q", hhReq.Parameters.Resolution)
	}
	if hhReq.Parameters.Duration != nil {
		t.Fatal("expected video-edit duration to be omitted")
	}
	if hhReq.Parameters.Ratio != "" {
		t.Fatalf("expected video-edit ratio to be omitted, got %q", hhReq.Parameters.Ratio)
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
}

func TestEstimateBillingUsesOfficialResolutionMultiplier(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"happyhorse-1.0-t2v","prompt":"city lights","size":"1080P","duration":5}`
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

func TestAdjustBillingOnCompleteUsesActualUsageAndGroupRatio(t *testing.T) {
	resp := HappyHorseResponse{
		Output: HappyHorseOutput{
			TaskID:     "upstream-task",
			TaskStatus: "SUCCEEDED",
			VideoURL:   "https://example.com/video.mp4",
		},
		Usage: &HappyHorseUsage{
			Duration: 5,
			SR:       1080,
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response failed: %v", err)
	}

	task := &model.Task{
		Data: data,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.14,
				GroupRatio: 1.5,
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

func TestParseTaskResultSupportsFloatUsageDuration(t *testing.T) {
	body := []byte(`{
		"output": {
			"task_id": "upstream-task",
			"task_status": "SUCCEEDED",
			"video_url": "https://example.com/video.mp4"
		},
		"usage": {
			"duration": 13.24,
			"input_video_duration": 6.62,
			"output_video_duration": 6.62,
			"video_count": 1,
			"SR": 720
		}
	}`)

	adaptor := &TaskAdaptor{}
	taskInfo, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("expected float usage duration to parse, got error: %v", err)
	}
	if taskInfo.Status != model.TaskStatusSuccess {
		t.Fatalf("expected success status, got %s", taskInfo.Status)
	}
}
