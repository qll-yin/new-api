package ali

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestValidateRequestAndSetActionAcceptsDashScopeNativeBodyOnUnifiedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"wan2.5-i2v-preview","input":{"prompt":"让图片动起来","img_url":"https://example.com/input.png","audio_url":"https://example.com/audio.mp3"},"parameters":{"resolution":"1080P","duration":5,"prompt_extend":false,"watermark":true,"audio":true,"seed":123}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("expected ali native body on unified route to pass, got error: %v", taskErr)
	}

	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		t.Fatalf("expected task request to be stored, got error: %v", err)
	}
	if taskReq.Prompt != "让图片动起来" {
		t.Fatalf("expected prompt from input.prompt, got %q", taskReq.Prompt)
	}

	aliReq, err := adaptor.convertToAliRequest(info, taskReq)
	if err != nil {
		t.Fatalf("expected ali request conversion to succeed, got error: %v", err)
	}
	if aliReq.Model != "wan2.5-i2v-preview" {
		t.Fatalf("expected model to be preserved, got %q", aliReq.Model)
	}
	if aliReq.Input.Prompt != "让图片动起来" {
		t.Fatalf("expected prompt to be preserved, got %q", aliReq.Input.Prompt)
	}
	if aliReq.Input.ImgURL != "https://example.com/input.png" {
		t.Fatalf("expected img_url to be preserved, got %q", aliReq.Input.ImgURL)
	}
	if aliReq.Input.AudioURL != "https://example.com/audio.mp3" {
		t.Fatalf("expected audio_url to be preserved, got %q", aliReq.Input.AudioURL)
	}
	if aliReq.Parameters == nil {
		t.Fatal("expected parameters to be present")
	}
	if aliReq.Parameters.Resolution != "1080P" || aliReq.Parameters.Duration != 5 {
		t.Fatalf("expected resolution/duration to be preserved, got %+v", aliReq.Parameters)
	}
	if aliReq.Parameters.PromptExtend || !aliReq.Parameters.Watermark || aliReq.Parameters.Audio == nil || !*aliReq.Parameters.Audio || aliReq.Parameters.Seed != 123 {
		t.Fatalf("expected remaining parameters to be preserved, got %+v", aliReq.Parameters)
	}
}

func TestTaskSubmitReqPreservesExplicitMetadataOverNativeInput(t *testing.T) {
	body := []byte(`{"model":"wan2.5-i2v-preview","prompt":"top prompt","input":{"prompt":"native prompt","img_url":"https://example.com/native.png"},"metadata":{"input":{"prompt":"metadata prompt","img_url":"https://example.com/metadata.png"},"parameters":{"resolution":"720P","duration":7}}}`)

	var taskReq relaycommon.TaskSubmitReq
	if err := common.Unmarshal(body, &taskReq); err != nil {
		t.Fatalf("expected task request to unmarshal, got error: %v", err)
	}
	if taskReq.Prompt != "top prompt" {
		t.Fatalf("expected explicit top-level prompt to win, got %q", taskReq.Prompt)
	}

	adaptor := &TaskAdaptor{}
	aliReq, err := adaptor.convertToAliRequest(&relaycommon.RelayInfo{}, taskReq)
	if err != nil {
		t.Fatalf("expected ali request conversion to succeed, got error: %v", err)
	}
	if aliReq.Input.Prompt != "metadata prompt" {
		t.Fatalf("expected explicit metadata input to win during provider conversion, got %q", aliReq.Input.Prompt)
	}
	if aliReq.Input.ImgURL != "https://example.com/metadata.png" {
		t.Fatalf("expected metadata img_url to win, got %q", aliReq.Input.ImgURL)
	}
	if aliReq.Parameters.Resolution != "720P" || aliReq.Parameters.Duration != 7 {
		t.Fatalf("expected metadata parameters to win, got %+v", aliReq.Parameters)
	}
}
