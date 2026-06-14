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
