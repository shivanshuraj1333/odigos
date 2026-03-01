package odigosurltemplateprocessor

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	deprecatedsemconv "go.opentelemetry.io/collector/semconv/v1.18.0"
	semconv "go.opentelemetry.io/collector/semconv/v1.27.0"
	"go.uber.org/zap"
)

// httpMethodOtherSpanName is the {method} placeholder for span name when
// http.request.method is _OTHER. Per semconv: "In other cases (when
// {http.request.method} is set to _OTHER), {method} MUST be HTTP."
// See https://opentelemetry.io/docs/specs/semconv/http/http-spans/#name
const httpMethodOtherSpanName = "HTTP"

// workloadRulesProvider returns URL templatization rules for the workload identified
// by resource attributes (from InstrumentationConfig via extension).
type workloadRulesProvider interface {
	GetWorkloadUrlTemplatizationRules(attrs pcommon.Map) ([]string, bool)
}

type urlTemplateProcessor struct {
	logger              *zap.Logger
	templatizationRules map[int][]TemplatizationRule // group templatization rules by segments length
	customIds           []internalCustomIdConfig

	excludeMatcher *PropertiesMatcher
	includeMatcher *PropertiesMatcher

	// workloadRulesProvider is set when workload_config_extension is configured; rules are derived from it at runtime.
	workloadRulesProvider workloadRulesProvider
}

func newUrlTemplateProcessor(set processor.Settings, config *Config) (*urlTemplateProcessor, error) {

	excludeMatcher := NewPropertiesMatcher(config.Exclude)
	includeMatcher := NewPropertiesMatcher(config.Include)

	parsedRules := map[int][]TemplatizationRule{}
	for _, rule := range config.TemplatizationRules {
		parsedRule, err := parseUserInputRuleString(rule)
		if err != nil {
			return nil, err
		}
		parsedRuleNumSegments := len(parsedRule)
		if _, ok := parsedRules[parsedRuleNumSegments]; !ok {
			parsedRules[parsedRuleNumSegments] = []TemplatizationRule{}
		}
		parsedRules[parsedRuleNumSegments] = append(parsedRules[parsedRuleNumSegments], parsedRule)
	}

	customIdsRegexp := make([]internalCustomIdConfig, 0, len(config.CustomIds))
	for _, ci := range config.CustomIds {
		regexpPattern, err := regexp.Compile(ci.Regexp)
		if err != nil {
			return nil, fmt.Errorf("invalid custom id regex: %w", err)
		}
		templateName := "id"
		if ci.TemplateName != "" {
			// if the template name is empty, we default to "id"
			templateName = ci.TemplateName
		}
		customIdsRegexp = append(customIdsRegexp, internalCustomIdConfig{
			Regexp: *regexpPattern,
			Name:   templateName,
		})
	}

	return &urlTemplateProcessor{
		logger:              set.Logger,
		templatizationRules: parsedRules,
		customIds:           customIdsRegexp,
		excludeMatcher:      excludeMatcher,
		includeMatcher:      includeMatcher,
	}, nil
}

func (p *urlTemplateProcessor) warnExtensionNotFound(extID string) {
	p.logger.Warn("url template processor: workload config extension not found; per-workload rules will not be applied",
		zap.String("extension_id", extID))
}

func (p *urlTemplateProcessor) warnExtensionWrongType(extID string) {
	p.logger.Warn("url template processor: extension has unexpected type; per-workload rules will not be applied", zap.String("extension_id", extID))
}

// parseRulesStrings parses rule strings (e.g. from extension) into the same structure used by the processor.
// Invalid rules are skipped with a warning log so one bad rule does not silently drop all others.
func (p *urlTemplateProcessor) parseRulesStrings(rules []string) map[int][]TemplatizationRule {
	parsed := make(map[int][]TemplatizationRule)
	for _, rule := range rules {
		parsedRule, err := parseUserInputRuleString(rule)
		if err != nil {
			p.logger.Warn("skipping invalid URL templatization rule from workload config", zap.String("rule", rule), zap.Error(err))
			continue
		}
		n := len(parsedRule)
		if _, ok := parsed[n]; !ok {
			parsed[n] = []TemplatizationRule{}
		}
		parsed[n] = append(parsed[n], parsedRule)
	}
	return parsed
}

func (p *urlTemplateProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		resourceSpans := td.ResourceSpans().At(i)
		res := resourceSpans.Resource()

		// Determine the active rule set for this resource/workload.
		// When the extension is configured, use per-workload rules from InstrumentationConfig.
		// The extension distinguishes three cases:
		//   ok=false            → workload not opted in; skip resource entirely.
		//   ok=true, len==0     → opted in but no explicit rules; ruleSet stays nil → heuristics apply.
		//   ok=true, len>0      → explicit rules; parse and apply, heuristics run for unmatched paths.
		// When no extension is configured, fall back to the static rules from processor config.
		var ruleSet map[int][]TemplatizationRule
		if p.workloadRulesProvider != nil {
			rules, ok := p.workloadRulesProvider.GetWorkloadUrlTemplatizationRules(res.Attributes())
			if !ok {
				// Workload is not opted in to URL templatization; leave its spans untouched.
				continue
			}
			if len(rules) > 0 {
				ruleSet = p.parseRulesStrings(rules)
			}
			// ok=true, len(rules)==0: opted in but no explicit rules → ruleSet stays nil → heuristics apply.
		} else {
			ruleSet = p.templatizationRules
		}

		// before processing the spans, first check if it should be processed according to the include/exclude matchers
		if p.excludeMatcher != nil && p.excludeMatcher.Match(res) {
			// always skip the resource spans if it matches the exclude matcher
			continue
		}
		// it doesn't make sense to have both include and exclude matchers, but we support it anyway
		if p.includeMatcher != nil && !p.includeMatcher.Match(res) {
			// if we have an include matcher, it must match the resource for it to be processed
			continue
		}
		// it is ok that both include and exclude matchers are nil, in that case we process all spans

		for j := 0; j < resourceSpans.ScopeSpans().Len(); j++ {
			scopeSpans := resourceSpans.ScopeSpans().At(j)
			for k := 0; k < scopeSpans.Spans().Len(); k++ {
				span := scopeSpans.Spans().At(k)
				p.processSpan(span, ruleSet)
			}
		}
	}
	return td, nil
}

func getHttpMethod(attr pcommon.Map) (string, bool) {
	// prefer to use the new "http.request.method" attribute
	if method, found := attr.Get(semconv.AttributeHTTPRequestMethod); found {
		return method.AsString(), true
	}
	// fallback to the old "http.method" attribute which might still be used
	// by some instrumentations.
	// TODO: remove this fallback in the future when all instrumentations are aligned with
	// update semantic conventions and no longer report "http.method"
	if method, found := attr.Get(deprecatedsemconv.AttributeHTTPMethod); found {
		return method.AsString(), true
	}
	return "", false
}

func getUrlPath(attr pcommon.Map) (string, bool) {
	// prefer the updated semantic convention "url.path" if available
	if urlPath, found := attr.Get(semconv.AttributeURLPath); found {
		return urlPath.AsString(), true
	}

	// fallback to the old "http.target" attribute which might still be used
	// by some instrumentations.
	// TODO: remove this fallback in the future when all instrumentations are aligned with
	// update semantic conventions and no longer report "http.target"
	if httpTarget, found := attr.Get(deprecatedsemconv.AttributeHTTPTarget); found {
		// the "http.target" attribute might contain a query string, so we need to
		// split it and only use the path part.
		// for example: "/user?id=123" => "/user"
		path := strings.SplitN(httpTarget.AsString(), "?", 2)[0]
		return path, true
	}
	return "", false
}

func getFullUrl(attr pcommon.Map) (string, bool) {
	// prefer the updated semantic convention "url.full" if available
	if fullUrl, found := attr.Get(semconv.AttributeURLFull); found {
		return fullUrl.AsString(), true
	}
	// fallback to the old "http.url" attribute which might still be used
	// by some instrumentations.
	// TODO: remove this fallback in the future when all instrumentations are aligned with
	// update semantic conventions and no longer report "http.url"
	if httpUrl, found := attr.Get(deprecatedsemconv.AttributeHTTPURL); found {
		return httpUrl.AsString(), true
	}
	return "", false
}

func (p *urlTemplateProcessor) applyTemplatizationOnPath(path string, ruleSet map[int][]TemplatizationRule) string {
	hasLeadingSlash := strings.HasPrefix(path, "/")
	if !hasLeadingSlash {
		path = "/" + path
	}

	inputPathSegments := strings.Split(path, "/")
	inputPathSegments = inputPathSegments[1:]
	if len(inputPathSegments) == 1 && inputPathSegments[0] == "" {
		// if the path is empty, we can't generate a templated url
		return "/" // always set a leading slash even if missing
	}

	rules, found := ruleSet[len(inputPathSegments)]
	if found {
		for _, rule := range rules {
			if templatedUrl, matched := attemptTemplateWithRule(inputPathSegments, rule); matched {
				if hasLeadingSlash {
					// if the path has a leading slash, we need to add it back
					templatedUrl = "/" + templatedUrl
				}
				return templatedUrl
			}
		}
	}

	templatedPath, isTemplated := defaultTemplatizeURLPath(inputPathSegments, p.customIds)
	if isTemplated {
		if hasLeadingSlash {
			// if the path has a leading slash, we need to add it back
			templatedPath = "/" + templatedPath
		}
		return templatedPath
	} else {
		// if no templated url is generated, we return the original path
		return path
	}
}

func (p *urlTemplateProcessor) calculateTemplatedUrlFromAttr(attr pcommon.Map, ruleSet map[int][]TemplatizationRule) (string, bool) {
	// this processor enhances url template value, which it extracts from full url or url path.
	// one of these is required for this processor to handle this span.
	urlPath, urlPathFound := getUrlPath(attr)
	if urlPathFound {
		// if url path is available, we can use it to generate the templated url
		// in case of query string, we only want the path part of the url (used with deprecated "http.target" attribute)
		templatedUrl := p.applyTemplatizationOnPath(urlPath, ruleSet)
		return templatedUrl, true
	}

	fullUrl, fullUrlFound := getFullUrl(attr)
	if fullUrlFound {
		parsed, err := url.Parse(fullUrl)
		if err != nil {
			// if we are unable to parse the url, we can't generate the templated url
			// so we skip this span
			return "", false
		}
		templatedUrl := p.applyTemplatizationOnPath(parsed.Path, ruleSet)
		return templatedUrl, true
	}

	return "", false
}

// updateHttpSpanName sets the span name to {method} {target} per HTTP semconv (Name section).
// See https://opentelemetry.io/docs/specs/semconv/http/http-spans/#name
// We only update when the current name looks like an HTTP-generated name (method only or
// "method path") so we do not overwrite custom span names.
func updateHttpSpanName(span ptrace.Span, httpMethod string, templatedUrl string) {
	currentName := span.Name()

	// {method} in span name: MUST be "HTTP" when http.request.method is _OTHER, else the attribute value.
	// Semconv: "The {method} MUST be {http.request.method} if the method represents the original
	// method known to the instrumentation. In other cases (when {http.request.method} is set to
	// _OTHER), {method} MUST be HTTP."
	methodForSpanName := httpMethod
	if httpMethod == "_OTHER" {
		methodForSpanName = httpMethodOtherSpanName
	}

	// Only update when the span name appears HTTP-generated: equals {method} or starts with "{method} ".
	// Semconv says instrumentations MUST NOT use URI path as target (high cardinality); names like
	// "GET /user/1234" are therefore non-compliant. We normalize them to "{method} {templated_target}".
	if currentName != methodForSpanName && !strings.HasPrefix(currentName, methodForSpanName+" ") {
		// Also allow raw attribute value so we can fix "_OTHER" or "_OTHER /path" if present
		if currentName != httpMethod && !strings.HasPrefix(currentName, httpMethod+" ") {
			return
		}
	}

	if templatedUrl == "" {
		return
	}

	// Semconv: "HTTP span names SHOULD be {method} {target} if there is a (low-cardinality) target available."
	// {target} is http.route (server) or url.template (client); we set it and the name together.
	newSpanName := fmt.Sprintf("%s %s", methodForSpanName, templatedUrl)
	span.SetName(newSpanName)
}

func (p *urlTemplateProcessor) enhanceSpan(span ptrace.Span, httpMethod string, targetAttribute string, ruleSet map[int][]TemplatizationRule) {
	attr := span.Attributes()

	// If the target attribute (http.route / url.template) is already set by the instrumentation,
	// do not overwrite it — but still normalize the span name using the existing value so that
	// high-cardinality names like "GET /items/3" become "GET /items/<int:item_id>".
	if val, found := attr.Get(targetAttribute); found {
		if val.Type() != pcommon.ValueTypeStr {
			// should not happen.
			return
		}
		existingVal := val.Str()
		if existingVal == "" {
			existingVal = "/"
		}
		updateHttpSpanName(span, httpMethod, existingVal)
		return
	}

	templatedUrl, found := p.calculateTemplatedUrlFromAttr(attr, ruleSet)
	if !found {
		// don't modify the span if we are unable to calculate the templated url
		return
	}

	// set the templated url in the target attribute and update the span name if needed
	attr.PutStr(targetAttribute, templatedUrl)
	updateHttpSpanName(span, httpMethod, templatedUrl)
}

func (p *urlTemplateProcessor) processSpan(span ptrace.Span, ruleSet map[int][]TemplatizationRule) {
	attr := span.Attributes()

	httpMethod, found := getHttpMethod(attr)
	if !found {
		// we only enhance http spans, so if there is no http.method attribute, we can skip it
		return
	}

	switch span.Kind() {
	case ptrace.SpanKindClient:
		// client spans write the url templated value in "url.template" attribute.
		p.enhanceSpan(span, httpMethod, semconv.AttributeURLTemplate, ruleSet)
	case ptrace.SpanKindServer:
		// server spans write the url templated value in "http.route" attribute.
		p.enhanceSpan(span, httpMethod, semconv.AttributeHTTPRoute, ruleSet)
	default:
		// http spans are either client or server
		// all other spans are ignored and never enhanced
		return
	}
}
