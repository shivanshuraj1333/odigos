package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// GuidedResponse is the standardized envelope every successful tool response
// uses. The point is to give the calling AI agent enough structure to act
// independently — not just data, but what the data means (context) and which
// follow-up tools likely make sense (next_steps). Wrap every success path
// through withGuidance.
type GuidedResponse struct {
	Data      any        `json:"data"`
	Context   string     `json:"context,omitempty"`
	NextSteps []NextStep `json:"next_steps,omitempty"`
}

// NextStep tells the agent "if you're trying to X, call tool Y with these args".
// Args are populated from the current response so the agent can chain without
// re-querying (e.g. pre-fill the workload triple).
type NextStep struct {
	When string         `json:"when"`
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
}

// withGuidance wraps data with optional context + next steps. ctx may be empty
// and steps may be nil for tools where guidance adds no value (rare).
func withGuidance(data any, ctx string, steps []NextStep) (*mcp.CallToolResult, error) {
	resp := GuidedResponse{Data: data, Context: ctx, NextSteps: steps}
	buf, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(buf)), nil
}

// errorWithOptions returns a tool error that names the valid choices so the
// agent can self-correct on the next call (cribl-mcp pattern). Use it for
// unknown enum / id / type values rather than a bare error string.
func errorWithOptions(msg string, valid []string) (*mcp.CallToolResult, error) {
	body := map[string]any{
		"error":         msg,
		"valid_options": valid,
	}
	buf, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(msg), nil
	}
	return mcp.NewToolResultText(string(buf)), nil
}
