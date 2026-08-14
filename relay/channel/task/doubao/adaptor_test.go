package doubao

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestConvertToRequestPayloadPreservesUsePersonCharacter(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-5-260628",
		Prompt: "test prompt",
		Metadata: map[string]interface{}{
			"use_person_character": true,
		},
	}

	body, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	if err != nil {
		t.Fatalf("convertToRequestPayload() error = %v", err)
	}
	if body.UsePersonCharacter == nil || !bool(*body.UsePersonCharacter) {
		t.Fatalf("UsePersonCharacter = %v, want true", body.UsePersonCharacter)
	}
}
