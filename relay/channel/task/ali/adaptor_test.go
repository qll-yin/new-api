package ali

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func testRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestValidateRequestAndSetActionAcceptsDashScopeNativeBodyOnUnifiedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"model":"wan2.5-i2v-preview","input":{"prompt":"让图片动起来","img_url":"https://example.com/input.png","audio_url":"https://example.com/audio.mp3"},"parameters":{"resolution":"1080P","duration":5,"prompt_extend":false,"watermark":true,"audio":true,"seed":123}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	adaptor := &TaskAdaptor{}
	info := testRelayInfo()
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
	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), taskReq)
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

func TestConvertToAliRequestWan27I2VBuildsMediaFromImage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:    "wan2.7-i2v",
		Prompt:   "animate the first frame",
		Image:    "https://example.com/first.png",
		Size:     "720p",
		Duration: 10,
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, "wan2.7-i2v", aliReq.Model)
	require.Equal(t, "720P", aliReq.Parameters.Resolution)
	require.Equal(t, 10, aliReq.Parameters.Duration)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
	}, aliReq.Input.Media)
	require.Empty(t, aliReq.Input.ImgURL)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"media"`)
	require.NotContains(t, string(body), `"img_url"`)
}

func TestConvertToAliRequestWan27I2VBuildsFirstAndLastFrameFromImages(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "interpolate between frames",
		Images: []string{
			"https://example.com/first.png",
			"https://example.com/last.png",
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VPrefersImageBeforeImagesAndInputReference(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "use the direct image",
		Image:          " https://example.com/direct.png ",
		Images:         []string{"https://example.com/images-first.png", " https://example.com/images-last.png "},
		InputReference: "https://example.com/input-reference.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/direct.png"},
		{Type: "last_frame", URL: "https://example.com/images-last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VFallsBackToFirstNonEmptyImage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "skip blank images",
		Image:  " ",
		Images: []string{
			" ",
			" https://example.com/first.png ",
			" https://example.com/last.png ",
		},
		InputReference: "https://example.com/input-reference.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VKeepsExplicitMetadataMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "continue the clip",
		Image:          "https://example.com/direct.png",
		Images:         []string{"https://example.com/images-first.png", "https://example.com/images-last.png"},
		InputReference: "https://example.com/input-reference.png",
		Metadata: map[string]interface{}{
			"input": map[string]interface{}{
				"media": []interface{}{
					map[string]interface{}{
						"type": "first_clip",
						"url":  "https://example.com/input.mp4",
					},
				},
			},
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_clip", URL: "https://example.com/input.mp4"},
	}, aliReq.Input.Media)
	require.Empty(t, aliReq.Input.ImgURL)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"media"`)
	require.NotContains(t, string(body), `"img_url"`)
}

func TestConvertToAliRequestWan27I2VRequiresMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "animate without a frame",
	}

	_, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "requires image"))
}

func TestConvertToAliRequestWan25I2VKeepsLegacyImgURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.5-i2v-preview",
		Prompt: "animate the first frame",
		Image:  "https://example.com/first.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, "https://example.com/first.png", aliReq.Input.ImgURL)
	require.Empty(t, aliReq.Input.Media)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"img_url"`)
	require.NotContains(t, string(body), `"media"`)
}
