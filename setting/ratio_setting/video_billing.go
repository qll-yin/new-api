package ratio_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

type VideoModelConfig struct {
	BaseResolution        string             `json:"base_resolution,omitempty"`
	ResolutionMultipliers map[string]float64 `json:"resolution_multipliers,omitempty"`
}

var videoModelConfigMap = types.NewRWMap[string, VideoModelConfig]()

func NormalizeVideoResolution(resolution string) string {
	normalized := strings.ToUpper(strings.TrimSpace(resolution))
	if normalized == "" {
		return ""
	}
	switch normalized {
	case "720", "1080", "480":
		return normalized + "P"
	}
	return normalized
}

func normalizeVideoModelConfig(cfg VideoModelConfig) VideoModelConfig {
	cfg.BaseResolution = NormalizeVideoResolution(cfg.BaseResolution)
	if len(cfg.ResolutionMultipliers) == 0 {
		cfg.ResolutionMultipliers = nil
		return cfg
	}

	normalizedMultipliers := make(map[string]float64, len(cfg.ResolutionMultipliers))
	for resolution, multiplier := range cfg.ResolutionMultipliers {
		normalizedResolution := NormalizeVideoResolution(resolution)
		if normalizedResolution == "" || multiplier <= 0 {
			continue
		}
		normalizedMultipliers[normalizedResolution] = multiplier
	}
	if len(normalizedMultipliers) == 0 {
		cfg.ResolutionMultipliers = nil
		return cfg
	}
	cfg.ResolutionMultipliers = normalizedMultipliers
	return cfg
}

func VideoModelConfig2JSONString() string {
	jsonBytes, err := common.Marshal(GetVideoModelConfigCopy())
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func UpdateVideoModelConfigByJSONString(jsonStr string) error {
	next := make(map[string]VideoModelConfig)
	if err := common.Unmarshal([]byte(jsonStr), &next); err != nil {
		return err
	}

	normalized := make(map[string]VideoModelConfig, len(next))
	for modelName, cfg := range next {
		normalized[FormatMatchingModelName(modelName)] = normalizeVideoModelConfig(cfg)
	}

	videoModelConfigMap.Clear()
	videoModelConfigMap.AddAll(normalized)
	InvalidateExposedDataCache()
	return nil
}

func GetVideoModelConfig(name string) (VideoModelConfig, bool) {
	name = FormatMatchingModelName(name)
	cfg, ok := videoModelConfigMap.Get(name)
	if !ok {
		cfg, ok = defaultVideoModelConfig[name]
	}
	if !ok {
		return VideoModelConfig{}, false
	}
	return normalizeVideoModelConfig(cfg), true
}

func GetVideoResolutionMultiplier(name, resolution string) (float64, string, bool) {
	cfg, ok := GetVideoModelConfig(name)
	if !ok {
		return 0, "", false
	}

	normalizedResolution := NormalizeVideoResolution(resolution)
	if normalizedResolution == "" {
		normalizedResolution = cfg.BaseResolution
	}
	if normalizedResolution == "" {
		return 1, "", true
	}
	if normalizedResolution == cfg.BaseResolution {
		return 1, normalizedResolution, true
	}

	multiplier, ok := cfg.ResolutionMultipliers[normalizedResolution]
	if !ok || multiplier <= 0 {
		return 1, normalizedResolution, true
	}
	return multiplier, normalizedResolution, true
}

func GetVideoModelConfigCopy() map[string]VideoModelConfig {
	return videoModelConfigMap.ReadAll()
}
