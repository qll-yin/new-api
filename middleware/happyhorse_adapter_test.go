package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestHappyHorseRequestConvertRewritesReusableBody(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/api/v1/services/aigc/video-generation/video-synthesis", HappyHorseRequestConvert(), func(c *gin.Context) {
		if c.Request.URL.Path != "/v1/video/generations" {
			t.Fatalf("expected rewritten path, got %q", c.Request.URL.Path)
		}
		if !c.GetBool("dashscope_native") {
			t.Fatal("expected dashscope_native flag to be set")
		}

		var req relaycommon.TaskSubmitReq
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			t.Fatalf("expected rewritten body to be reusable, got error: %v", err)
		}
		if req.Model != "happyhorse-1.0-t2v" {
			t.Fatalf("expected model to be preserved, got %q", req.Model)
		}
		if req.Prompt != "make a running horse" {
			t.Fatalf("expected prompt to be rewritten from input.prompt, got %q", req.Prompt)
		}
		if req.Metadata == nil {
			t.Fatal("expected metadata to be preserved")
		}
		c.Status(http.StatusNoContent)
	})

	body := `{"model":"happyhorse-1.0-t2v","input":{"prompt":"make a running horse"},"parameters":{"resolution":"720P","duration":5}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusNoContent, recorder.Code, recorder.Body.String())
	}
}

func TestHappyHorseRequestConvertPreservesLegacyInputForImage2Video(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/api/v1/services/aigc/image2video/video-synthesis", HappyHorseRequestConvert(), func(c *gin.Context) {
		if c.Request.URL.Path != "/v1/video/generations" {
			t.Fatalf("expected rewritten path, got %q", c.Request.URL.Path)
		}
		if !c.GetBool("dashscope_native") {
			t.Fatal("expected dashscope_native flag to be set")
		}

		var req relaycommon.TaskSubmitReq
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			t.Fatalf("expected rewritten body to be reusable, got error: %v", err)
		}
		if req.Model != "wan2.2-animate-move" {
			t.Fatalf("expected model to be preserved, got %q", req.Model)
		}
		if req.Metadata == nil {
			t.Fatal("expected metadata to be preserved")
		}
		input, ok := req.Metadata["input"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected input metadata to be preserved, got %#v", req.Metadata["input"])
		}
		if input["image_url"] != "https://example.com/image.png" {
			t.Fatalf("expected image_url to be preserved, got %v", input["image_url"])
		}
		if input["video_url"] != "https://example.com/video.mp4" {
			t.Fatalf("expected video_url to be preserved, got %v", input["video_url"])
		}
		c.Status(http.StatusNoContent)
	})

	body := `{"model":"wan2.2-animate-move","input":{"image_url":"https://example.com/image.png","video_url":"https://example.com/video.mp4","watermark":false},"parameters":{"mode":"wan-pro"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/image2video/video-synthesis", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusNoContent, recorder.Code, recorder.Body.String())
	}
}
