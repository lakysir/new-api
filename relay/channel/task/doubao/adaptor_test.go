package doubao

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestConvertToRequestPayloadPreservesToolParameters(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-5-260628",
		Prompt: "test prompt",
		Metadata: map[string]interface{}{
			"tools": []interface{}{
				map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name": "search",
						"parameters": map[string]interface{}{
							"type": "object",
						},
					},
				},
			},
		},
	}

	body, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	if err != nil {
		t.Fatalf("convertToRequestPayload() error = %v", err)
	}
	if len(body.Tools) != 1 {
		t.Fatalf("Tools length = %d, want 1", len(body.Tools))
	}
	function, ok := body.Tools[0]["function"].(map[string]interface{})
	if !ok {
		t.Fatalf("Tools[0].function = %#v, want nested object", body.Tools[0]["function"])
	}
	if function["name"] != "search" {
		t.Fatalf("Tools[0].function.name = %#v, want search", function["name"])
	}
	parameters, ok := function["parameters"].(map[string]interface{})
	if !ok || parameters["type"] != "object" {
		t.Fatalf("Tools[0].function.parameters = %#v, want object schema", function["parameters"])
	}
}

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
