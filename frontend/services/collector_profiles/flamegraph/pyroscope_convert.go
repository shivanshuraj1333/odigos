// Pyroscope OTLP conversion: github.com/grafana/pyroscope/pkg/ingester/otlp.ConvertOtelToGoogle
// (same code path as Grafana Pyroscope ingest). Used only via SamplesFromOTLPChunk.
package flamegraph

import (
	"fmt"
	"reflect"
	"unsafe"

	"google.golang.org/protobuf/encoding/protojson"

	pprofileotlp "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	otelProfile "go.opentelemetry.io/proto/otlp/profiles/v1development"

	"github.com/grafana/pyroscope/pkg/ingester/otlp"
	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

var protoJSONUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// tryPyroscopeOTLP parses OTLP profile JSON as ExportProfilesServiceRequest and converts via
// Pyroscope's ConvertOtelToGoogle. Returns ok=false with a short reason when this path cannot be used.
func tryPyroscopeOTLP(chunk []byte) (samples []Sample, ok bool, failReason string) {
	req := &pprofileotlp.ExportProfilesServiceRequest{}
	if err := protoJSONUnmarshal.Unmarshal(chunk, req); err != nil {
		return nil, false, fmt.Sprintf("protojson_unmarshal: %v", err)
	}
	if req.Dictionary == nil || len(req.Dictionary.StringTable) == 0 {
		return nil, false, "missing_or_empty_dictionary_string_table"
	}
	if len(req.ResourceProfiles) == 0 {
		return nil, false, "no_resource_profiles"
	}
	var out []Sample
	for _, rp := range req.ResourceProfiles {
		if rp.ScopeProfiles == nil {
			continue
		}
		for _, sp := range rp.ScopeProfiles {
			if sp.Profiles == nil {
				continue
			}
			for _, p := range sp.Profiles {
				out = append(out, convertProfileViaPyroscope(p, req.Dictionary)...)
			}
		}
	}
	if len(out) == 0 {
		return nil, false, "convert_otel_to_google_yielded_no_samples"
	}
	return out, true, ""
}

// convertProfileViaPyroscope runs Pyroscope's OTLP→pprof conversion and turns the result into our Sample slice.
func convertProfileViaPyroscope(src *otelProfile.Profile, dictionary *otelProfile.ProfilesDictionary) []Sample {
	converted, err := otlp.ConvertOtelToGoogle(src, dictionary)
	if err != nil {
		return nil
	}
	var out []Sample
	for _, cp := range converted {
		cp := cp // local copy so &cp is addressable for unexported field reflection
		p := extractGoogleProfile(&cp)
		if p != nil {
			out = append(out, googleProfileToSamples(p)...)
		}
	}
	return out
}

// extractGoogleProfile reads the unexported .profile field from otlp.convertedProfile via reflection.
// cp must be *convertedProfile (or a pointer to the struct value); map iteration yields non-addressable
// copies, so callers must pass &cp from a local variable.
func extractGoogleProfile(cp interface{}) *googleProfile.Profile {
	v := reflect.ValueOf(cp)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName("profile")
	if !f.IsValid() {
		return nil
	}
	var p *googleProfile.Profile
	if f.CanInterface() {
		p, _ = f.Interface().(*googleProfile.Profile)
	} else if f.CanAddr() {
		p = *(**googleProfile.Profile)(unsafe.Pointer(f.Addr().UnsafePointer()))
	}
	return p
}

// googleProfileToSamples converts a Google pprof Profile to our Sample format (root-first stack, value).
func googleProfileToSamples(p *googleProfile.Profile) []Sample {
	if p == nil || len(p.Sample) == 0 || len(p.StringTable) == 0 {
		return nil
	}
	locByID := make(map[uint64]*googleProfile.Location)
	for _, loc := range p.Location {
		locByID[loc.Id] = loc
	}
	funcByID := make(map[uint64]*googleProfile.Function)
	for _, fn := range p.Function {
		funcByID[fn.Id] = fn
	}
	getName := func(stringIdx int64) string {
		if stringIdx >= 0 && int(stringIdx) < len(p.StringTable) {
			return p.StringTable[stringIdx]
		}
		return ""
	}
	var out []Sample
	for _, s := range p.Sample {
		var value int64
		for _, v := range s.Value {
			value += v
		}
		if value <= 0 {
			value = 1
		}
		stack := make([]string, 0, len(s.LocationId))
		for i := len(s.LocationId) - 1; i >= 0; i-- {
			locID := s.LocationId[i]
			loc := locByID[locID]
			if loc == nil || len(loc.Line) == 0 {
				continue
			}
			line := loc.Line[0]
			fn := funcByID[line.FunctionId]
			if fn == nil {
				continue
			}
			name := getName(fn.Name)
			if name != "" {
				stack = append(stack, name)
			}
		}
		if len(stack) > 0 {
			out = append(out, Sample{Stack: stack, Value: value})
		}
	}
	return out
}
