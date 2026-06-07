package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// args.go contains the small parsing helpers every handler uses to pull values
// out of the MCP request bag. They consistently distinguish "missing/unset"
// from a zero value where it matters (ptr-returning variants), and they fall
// back gracefully when the request has no arguments at all.

// args returns the request argument map or nil if absent / wrong type.
func args(req mcp.CallToolRequest) map[string]any {
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return nil
}

func optString(req mcp.CallToolRequest, key, def string) string {
	if v, err := req.RequireString(key); err == nil {
		return v
	}
	return def
}

func optBool(req mcp.CallToolRequest, key string, def bool) bool {
	a := args(req)
	if a == nil {
		return def
	}
	if v, ok := a[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

// optBoolPtr returns a *bool only if the caller explicitly provided the key.
// Needed where "unset" must be distinguished from "false" (most CodeAttributes
// / TraceConfig fields).
func optBoolPtr(req mcp.CallToolRequest, key string) *bool {
	a := args(req)
	if a == nil {
		return nil
	}
	if v, ok := a[key]; ok {
		if b, ok := v.(bool); ok {
			return &b
		}
	}
	return nil
}

func optInt(req mcp.CallToolRequest, key string, def int) int {
	a := args(req)
	if a == nil {
		return def
	}
	if v, ok := a[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return def
}

func optInt64Ptr(req mcp.CallToolRequest, key string) *int64 {
	a := args(req)
	if a == nil {
		return nil
	}
	if v, ok := a[key]; ok {
		if f, ok := v.(float64); ok {
			i := int64(f)
			return &i
		}
	}
	return nil
}

func optFloat64(req mcp.CallToolRequest, key string, def float64) float64 {
	a := args(req)
	if a == nil {
		return def
	}
	if v, ok := a[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return def
}

func optStringSlice(req mcp.CallToolRequest, key string) []string {
	if s, err := req.RequireStringSlice(key); err == nil {
		return s
	}
	return nil
}

// requireStringMap parses an object arg whose values are all strings.
func requireStringMap(req mcp.CallToolRequest, key string) (map[string]string, error) {
	a := args(req)
	if a == nil {
		return nil, fmt.Errorf("missing arguments")
	}
	raw, ok := a[key]
	if !ok {
		return nil, fmt.Errorf("missing required parameter %q", key)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parameter %q must be an object of string->string", key)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("field %q must be a string", k)
		}
		out[k] = s
	}
	return out, nil
}

// decodeArg JSON round-trips a raw argument into a typed Go struct so we
// accept complex nested config (sampler filters, action configs) and reuse the
// real API types for validation rather than hand-mapping every field. Returns
// false if the key was absent.
func decodeArg(req mcp.CallToolRequest, key string, out any) (bool, error) {
	a := args(req)
	if a == nil {
		return false, nil
	}
	raw, ok := a[key]
	if !ok || raw == nil {
		return false, nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(buf, out); err != nil {
		return false, fmt.Errorf("invalid %q: %w", key, err)
	}
	return true, nil
}
