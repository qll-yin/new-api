package taskcommon

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func FindVideoResolutionMultiplier(otherRatios map[string]float64) float64 {
	for key, value := range otherRatios {
		if strings.HasPrefix(key, "resolution-") && value > 0 {
			return value
		}
	}
	return 1
}

func ResolveVideoModelPrice(modelPrice float64, otherRatios map[string]float64) float64 {
	if modelPrice <= 0 {
		return 0
	}
	return modelPrice * FindVideoResolutionMultiplier(otherRatios)
}

func resolveVideoModelName(task *model.Task) string {
	if task == nil {
		return ""
	}
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	if task.Properties.OriginModelName != "" {
		return task.Properties.OriginModelName
	}
	return task.Properties.UpstreamModelName
}

func EstimateVideoOtherRatios(modelName string, durationSeconds int, resolution string) map[string]float64 {
	if durationSeconds <= 0 {
		return nil
	}

	multiplier, normalizedResolution, ok := ratio_setting.GetVideoResolutionMultiplier(modelName, resolution)
	if !ok {
		return nil
	}

	otherRatios := map[string]float64{
		"seconds": float64(durationSeconds),
	}
	if normalizedResolution != "" {
		otherRatios[fmt.Sprintf("resolution-%s", normalizedResolution)] = multiplier
	}
	return otherRatios
}

func FindVideoResolutionFromOtherRatios(otherRatios map[string]float64) string {
	for key := range otherRatios {
		if strings.HasPrefix(key, "resolution-") {
			return strings.TrimPrefix(key, "resolution-")
		}
	}
	return ""
}

func CalculateVideoTaskQuota(task *model.Task, durationSeconds float64, resolution string) int {
	if task == nil || task.PrivateData.BillingContext == nil || durationSeconds <= 0 {
		return 0
	}

	billingContext := task.PrivateData.BillingContext
	resolvedUnitPrice := billingContext.ResolvedModelPrice
	if resolvedUnitPrice <= 0 && billingContext.ModelPrice <= 0 {
		return 0
	}

	modelName := resolveVideoModelName(task)
	if resolvedUnitPrice <= 0 {
		if modelName == "" {
			resolvedUnitPrice = ResolveVideoModelPrice(billingContext.ModelPrice, billingContext.OtherRatios)
		} else {
			multiplier, normalizedResolution, ok := ratio_setting.GetVideoResolutionMultiplier(
				modelName,
				resolution,
			)
			if !ok {
				normalizedResolution = FindVideoResolutionFromOtherRatios(billingContext.OtherRatios)
				if normalizedResolution != "" {
					if v, exists := billingContext.OtherRatios[fmt.Sprintf("resolution-%s", normalizedResolution)]; exists && v > 0 {
						multiplier = v
						ok = true
					}
				}
				if !ok {
					multiplier, normalizedResolution, ok = ratio_setting.GetVideoResolutionMultiplier(
						modelName,
						normalizedResolution,
					)
					if !ok {
						resolvedUnitPrice = ResolveVideoModelPrice(billingContext.ModelPrice, billingContext.OtherRatios)
					}
				}
			}
			if resolvedUnitPrice <= 0 && multiplier > 0 {
				resolvedUnitPrice = billingContext.ModelPrice * multiplier
			}
		}
	}
	if resolvedUnitPrice <= 0 {
		return 0
	}

	groupRatio := billingContext.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1
	}

	cost := resolvedUnitPrice * durationSeconds
	return billingexpr.QuotaRound(cost * common.QuotaPerUnit * groupRatio)
}
