package taskcommon

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func TestEstimateVideoOtherRatios(t *testing.T) {
	ratios := EstimateVideoOtherRatios("happyhorse-1.0-t2v", 5, "1080P")
	if got := ratios["seconds"]; got != 5 {
		t.Fatalf("expected seconds=5, got %v", got)
	}
	want := 0.24 / 0.14
	if got := ratios["resolution-1080P"]; got != want {
		t.Fatalf("expected resolution multiplier %v, got %v", want, got)
	}
}

func TestCalculateVideoTaskQuotaFallsBackToStoredRatios(t *testing.T) {
	task := &model.Task{
		Properties: model.Properties{
			OriginModelName: "happyhorse-1.0-t2v",
		},
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

	got := CalculateVideoTaskQuota(task, 5, "1080P")
	want := int(0.14 * 5 * (0.24 / 0.14) * common.QuotaPerUnit * 1.5)
	if got != want {
		t.Fatalf("expected quota %d, got %d", want, got)
	}
}
