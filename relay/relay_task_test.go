package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyTaskParamOverrideAppliesToFinalRequestBody(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"path":  "parameters.extra",
						"mode":  "set",
						"value": "enabled",
					},
				},
			},
		},
	}

	reader, err := applyTaskParamOverride(strings.NewReader(`{"model":"happyhorse","parameters":{}}`), info)
	require.NoError(t, err)

	body, err := io.ReadAll(reader)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, common.Unmarshal(body, &got))
	parameters, ok := got["parameters"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "enabled", parameters["extra"])
}

func TestResetTaskStatusCodeUsesChannelMapping(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("status_code_mapping", `{"400":429}`)

	taskErr := &dto.TaskError{StatusCode: http.StatusBadRequest}
	resetTaskStatusCode(ctx, taskErr)

	require.Equal(t, http.StatusTooManyRequests, taskErr.StatusCode)
}
func TestApplyTaskOtherRatiosToQuotaRoundsFinalQuota(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 70000,
		},
	}
	got, ok := recalcQuotaFromRatios(info, map[string]float64{
		"seconds":          10,
		"resolution-1080P": 1.2857142857142858,
	})

	require.True(t, ok)
	require.Equal(t, 900000, got)
}
