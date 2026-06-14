package ratio_setting

import "testing"

func TestNormalizeVideoResolution(t *testing.T) {
	tests := map[string]string{
		"":     "",
		"720":  "720P",
		"1080": "1080P",
		"480p": "480P",
		"2k":   "2K",
	}

	for input, want := range tests {
		if got := NormalizeVideoResolution(input); got != want {
			t.Fatalf("NormalizeVideoResolution(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGetVideoResolutionMultiplierFallsBackToDefaults(t *testing.T) {
	multiplier, resolution, ok := GetVideoResolutionMultiplier("happyhorse-1.0-t2v", "1080")
	if !ok {
		t.Fatal("expected default happyhorse config to be available")
	}
	if resolution != "1080P" {
		t.Fatalf("expected normalized resolution 1080P, got %q", resolution)
	}
	want := 0.24 / 0.14
	if multiplier != want {
		t.Fatalf("expected multiplier %v, got %v", want, multiplier)
	}
}

func TestGetVideoResolutionMultiplierUsesBaseResolutionByDefault(t *testing.T) {
	multiplier, resolution, ok := GetVideoResolutionMultiplier("happyhorse-1.0-t2v", "")
	if !ok {
		t.Fatal("expected default happyhorse config to be available")
	}
	if resolution != "720P" {
		t.Fatalf("expected base resolution 720P, got %q", resolution)
	}
	if multiplier != 1 {
		t.Fatalf("expected base resolution multiplier 1, got %v", multiplier)
	}
}
