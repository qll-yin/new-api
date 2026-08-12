package ali

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func TestQwenWanImageNativeInputAndParametersRemainInImageExtra(t *testing.T) {
	body := []byte(`{"model":"wan2.7-image","prompt":"画一只猫","input":{"messages":[{"role":"user","content":[{"text":"画一只猫"}]}]},"parameters":{"size":"2K","n":2,"watermark":true}}`)

	var request dto.ImageRequest
	if err := common.Unmarshal(body, &request); err != nil {
		t.Fatalf("expected image request to unmarshal, got error: %v", err)
	}
	if _, ok := request.Extra["input"]; !ok {
		t.Fatal("expected native input to remain in ImageRequest.Extra")
	}
	if _, ok := request.Extra["parameters"]; !ok {
		t.Fatal("expected native parameters to remain in ImageRequest.Extra")
	}

	info := &relaycommon.RelayInfo{
		OriginModelName: "wan2.7-image",
		PriceData:       types.PriceData{},
	}
	aliReq, err := oaiImage2AliImageRequest(info, request, true)
	if err != nil {
		t.Fatalf("expected image conversion to succeed, got error: %v", err)
	}
	if aliReq.Input == nil || len(aliReq.Input.Messages) != 1 {
		t.Fatalf("expected native input messages to be preserved, got %+v", aliReq.Input)
	}
	if aliReq.Parameters.Size != "2K" || aliReq.Parameters.N != 2 || !aliReq.Parameters.Watermark {
		t.Fatalf("expected native parameters to be preserved, got %+v", aliReq.Parameters)
	}
}
