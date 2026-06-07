package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWithGuidanceShape(t *testing.T) {
	res, err := withGuidance(map[string]any{"x": 1},
		"because reasons",
		[]NextStep{{When: "always", Tool: "next_tool", Args: map[string]any{"y": 2}}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("nil/empty result")
	}
	// First content block is text JSON.
	var body GuidedResponse
	tc, ok := res.Content[0].(interface{ GetType() string })
	_ = tc
	_ = ok
	// fall back to JSON-marshal/parse round trip
	buf, _ := json.Marshal(res.Content[0])
	var raw map[string]any
	_ = json.Unmarshal(buf, &raw)
	text, _ := raw["text"].(string)
	if !strings.Contains(text, `"because reasons"`) || !strings.Contains(text, `"next_tool"`) {
		t.Fatalf("guidance not in text body; got %s", text)
	}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("body parse: %v", err)
	}
	if body.Context != "because reasons" {
		t.Fatalf("ctx mismatch: %q", body.Context)
	}
	if len(body.NextSteps) != 1 || body.NextSteps[0].Tool != "next_tool" {
		t.Fatalf("next_steps mismatch: %+v", body.NextSteps)
	}
}

func TestErrorWithOptions(t *testing.T) {
	res, _ := errorWithOptions("bad enum", []string{"a", "b"})
	buf, _ := json.Marshal(res.Content[0])
	var raw map[string]any
	_ = json.Unmarshal(buf, &raw)
	text, _ := raw["text"].(string)
	if !strings.Contains(text, `"valid_options"`) || !strings.Contains(text, `"a"`) {
		t.Fatalf("valid_options missing: %s", text)
	}
}

func TestWithGuidanceOmitsEmpty(t *testing.T) {
	res, _ := withGuidance("hi", "", nil)
	buf, _ := json.Marshal(res.Content[0])
	var raw map[string]any
	_ = json.Unmarshal(buf, &raw)
	text, _ := raw["text"].(string)
	if strings.Contains(text, "next_steps") || strings.Contains(text, "context") {
		t.Fatalf("omitempty broken: %s", text)
	}
}
