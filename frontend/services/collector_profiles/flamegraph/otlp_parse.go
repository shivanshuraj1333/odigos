package flamegraph

import (
	"encoding/json"
	"fmt"
)

// ParsedChunk holds extracted samples and name lookup from one OTLP/JSON chunk.
type ParsedChunk struct {
	Names   map[int]string // location/frame index -> symbol name
	Samples []Sample       // each sample: stack (root-first) and value
}

// Sample is one profile sample: stack of frame names (root first) and value (e.g. count).
type Sample struct {
	Stack []string
	Value int64
}

// ParseOTLPChunk parses one OTLP/JSON profile chunk (as produced by pprofile.JSONMarshaler)
// and returns samples with resolved stack names. Handles camelCase and snake_case keys.
func ParseOTLPChunk(data []byte) (*ParsedChunk, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	names := make(map[int]string)
	var samples []Sample
	extractSamplesAndNames(raw, &samples, names)
	return &ParsedChunk{Names: names, Samples: samples}, nil
}

func getKey(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	}
	return 0
}

func extractSamplesAndNames(obj interface{}, samples *[]Sample, names map[int]string) {
	if obj == nil {
		return
	}
	m, ok := obj.(map[string]interface{})
	if !ok {
		return
	}
	if data := getKey(m, "data", "Data"); data != nil {
		if dm, ok := data.(map[string]interface{}); ok {
			extractSamplesAndNames(dm, samples, names)
			return
		}
	}
	// Always fill names from this object first. OTLP puts stringTable/location/function at SCOPE level;
	// when we recurse into "profiles" we only see profile objects with samples. So we must read names
	// from the current (scope) object before recursing into profiles, otherwise we get frame_X.
	extractNamesFromObject(m, names)

	if rps := getKey(m, "resourceProfiles", "ResourceProfiles", "resource_profiles"); rps != nil {
		if arr, ok := rps.([]interface{}); ok {
			for _, rp := range arr {
				if rpm, ok := rp.(map[string]interface{}); ok {
					extractSamplesAndNames(rpm, samples, names)
				}
			}
			return
		}
	}
	if scopes := getKey(m, "scopeProfiles", "ScopeProfiles", "scope_profiles"); scopes != nil {
		if arr, ok := scopes.([]interface{}); ok {
			for _, s := range arr {
				if sm, ok := s.(map[string]interface{}); ok {
					extractSamplesAndNames(sm, samples, names)
				}
			}
			return
		}
	}
	if profs := getKey(m, "profiles", "Profiles"); profs != nil {
		if arr, ok := profs.([]interface{}); ok {
			for _, p := range arr {
				if pm, ok := p.(map[string]interface{}); ok {
					extractSamplesAndNames(pm, samples, names)
				}
			}
			return
		}
	}
	// Process samples (names already filled from this object or parent scope via extractNamesFromObject)
	if sampleArr := getKey(m, "samples", "Samples", "sample", "Sample"); sampleArr != nil {
		if arr, ok := sampleArr.([]interface{}); ok {
			for _, s := range arr {
				so, ok := s.(map[string]interface{})
				if !ok {
					continue
				}
				locIDs := getSampleLocIDs(so)
				value := getSampleValue(so)
				if value <= 0 && len(locIDs) == 0 {
					continue
				}
				if value <= 0 {
					value = 1
				}
				stack := make([]string, 0, len(locIDs))
				for _, id := range locIDs {
					if name, ok := names[id]; ok && name != "" {
						stack = append(stack, name)
					} else {
						stack = append(stack, fmt.Sprintf("frame_%d", id))
					}
				}
				*samples = append(*samples, Sample{Stack: stack, Value: value})
			}
		}
		return
	}
	// Recurse into nested objects that might contain profiles
	for _, key := range []string{"resource", "scope", "profile", "Profile"} {
		if v := m[key]; v != nil {
			if vm, ok := v.(map[string]interface{}); ok {
				extractSamplesAndNames(vm, samples, names)
			}
		}
	}
}

// extractNamesFromObject fills names from stringTable, location, function in this object.
// Call this before recursing into "profiles" so scope-level dictionary is used when parsing profile samples.
func extractNamesFromObject(m map[string]interface{}, names map[int]string) {
	if st := getKey(m, "stringTable", "StringTable", "string_table"); st != nil {
		if arr, ok := st.([]interface{}); ok {
			for i, v := range arr {
				if s, ok := v.(string); ok {
					names[i] = s
				}
			}
		}
	}
	if locs := getKey(m, "location", "Location", "locations"); locs != nil {
		if arr, ok := locs.([]interface{}); ok {
			for idx, loc := range arr {
				name := resolveLocationName(loc, names)
				if name != "" {
					names[idx] = name
				} else {
					names[idx] = fmt.Sprintf("loc_%d", idx)
				}
			}
		}
	}
	if fncs := getKey(m, "function", "Function", "functions"); fncs != nil {
		if arr, ok := fncs.([]interface{}); ok {
			for idx, fn := range arr {
				name := resolveFunctionName(fn, names)
				if name != "" {
					names[idx] = name
				}
			}
		}
	}
}

func resolveLocationName(loc interface{}, names map[int]string) string {
	lm, ok := loc.(map[string]interface{})
	if !ok {
		return ""
	}
	if nameRef := getKey(lm, "name", "Name", "functionName", "function_name"); nameRef != nil {
		if idx, ok := toInt(nameRef); ok && idx >= 0 {
			return names[idx]
		}
	}
	if lineArr := getKey(lm, "line", "Line"); lineArr != nil {
		if arr, ok := lineArr.([]interface{}); ok && len(arr) > 0 {
			first := arr[0]
			if fm, ok := first.(map[string]interface{}); ok {
				if funcIdx := getKey(fm, "functionIndex", "FunctionIndex", "function_index"); funcIdx != nil {
					if idx, ok := toInt(funcIdx); ok && idx >= 0 {
						return names[idx]
					}
				}
			}
		}
	}
	return ""
}

func resolveFunctionName(fn interface{}, names map[int]string) string {
	fm, ok := fn.(map[string]interface{})
	if !ok {
		return ""
	}
	if nameRef := getKey(fm, "name", "Name"); nameRef != nil {
		if idx, ok := toInt(nameRef); ok && idx >= 0 {
			return names[idx]
		}
	}
	return ""
}

func toInt(v interface{}) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	}
	return 0, false
}

func getSampleLocIDs(so map[string]interface{}) []int {
	if locArray := getKey(so, "attributeIndices", "attribute_indices", "locationIdList", "location_id_list"); locArray != nil {
		if arr, ok := locArray.([]interface{}); ok {
			ids := make([]int, 0, len(arr))
			for i := len(arr) - 1; i >= 0; i-- {
				if idx, ok := toInt(arr[i]); ok {
					ids = append(ids, idx)
				}
			}
			return ids
		}
	}
	if locID := getKey(so, "locationId", "LocationId", "location_id"); locID != nil {
		if idx, ok := toInt(locID); ok {
			return []int{idx}
		}
	}
	if stackIdx := getKey(so, "stackIndex", "stack_index"); stackIdx != nil {
		if idx, ok := toInt(stackIdx); ok {
			return []int{idx}
		}
	}
	return nil
}

func getSampleValue(so map[string]interface{}) int64 {
	if v := getKey(so, "value", "Value", "values"); v != nil {
		if n, ok := v.(float64); ok {
			return int64(n)
		}
		if arr, ok := v.([]interface{}); ok && len(arr) > 0 {
			var sum int64
			for _, a := range arr {
				sum += toInt64(a)
			}
			return sum
		}
	}
	if ts := getKey(so, "timestampsUnixNano", "timestamps_unix_nano"); ts != nil {
		if arr, ok := ts.([]interface{}); ok {
			return int64(len(arr))
		}
	}
	return 1
}
