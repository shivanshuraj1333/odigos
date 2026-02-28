package odigosurltemplateprocessor

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	deprecatedsemconv "go.opentelemetry.io/collector/semconv/v1.18.0"
	semconv "go.opentelemetry.io/collector/semconv/v1.27.0"
	"go.uber.org/zap"
)

// workloadConfigProvider is implemented by odigosworkloadconfigextension.
// It provides per-workload URL templatization rules keyed by resource attributes.
// Returns (rules, true) when the workload is configured for URL templatization
// (rules may be empty, meaning only default heuristics apply).
// Returns (nil, false) when the workload has no URL templatization configuration.
type workloadConfigProvider interface {
	GetWorkloadUrlTemplatizationRules(attrs pcommon.Map) ([]string, bool)
}

type urlTemplateProcessor struct {
	logger              *zap.Logger
	templatizationRules map[int][]TemplatizationRule // group templatization rules by segments length
	customIds           []internalCustomIdConfig

	excludeMatcher *PropertiesMatcher
	includeMatcher *PropertiesMatcher

	// workloadConfigProvider is optionally set in Start() when odigosworkloadconfigextension
	// is present. When set, per-workload rules override the static templatizationRules.
	workloadConfigProvider workloadConfigProvider
}

// Start is called by the collector when the processor is started.
// It searches all running extensions for one that implements workloadConfigProvider
// (i.e. odigosworkloadconfigextension). If found, per-workload rules will be used
// instead of the static templatizationRules for that workload.
func (p *urlTemplateProcessor) Start(_ context.Context, host component.Host) error {
	for _, ext := range host.GetExtensions() {
		if provider, ok := ext.(workloadConfigProvider); ok {
			p.workloadConfigProvider = provider
			p.logger.Info("odigosurltemplateprocessor: found workload config extension, will use per-workload URL templatization rules")
			break
		}
	}
	return nil
}

// resolveRulesForResource returns the templatization rules to apply for the given resource.
// When a workload config extension is available:
//   - If the workload has URL templatization configured → return its specific rules.
//   - If the workload is NOT configured → return nil, false (skip templatization).
//
// When no extension is available, falls back to the static templatizationRules.
func (p *urlTemplateProcessor) resolveRulesForResource(attrs pcommon.Map) (map[int][]TemplatizationRule, bool) {
	if p.workloadConfigProvider == nil {
		// No extension: use static config. An empty static config means no templatization.
		return p.templatizationRules, true
	}

	ruleStrings, participating := p.workloadConfigProvider.GetWorkloadUrlTemplatizationRules(attrs)
	if !participating {
		// Workload not configured for URL templatization — skip it entirely.
		return nil, false
	}

	// Parse the per-workload rule strings into the internal representation.
	parsedRules := map[int][]TemplatizationRule{}
	for _, ruleStr := range ruleStrings {
		parsedRule, err := parseUserInputRuleString(ruleStr)
		if err != nil {
			p.logger.Error("odigosurltemplateprocessor: invalid per-workload URL templatization rule, skipping",
				zap.String("rule", ruleStr), zap.Error(err))
			continue
		}
		n := len(parsedRule)
		parsedRules[n] = append(parsedRules[n], parsedRule)
	}
	return parsedRules, true
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

func (p *urlTemplateProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		resourceSpans := td.ResourceSpans().At(i)

		// before processing the spans, first check if it should be processed according to the include/exclude matchers
		if p.excludeMatcher != nil && p.excludeMatcher.Match(resourceSpans.Resource()) {
			// always skip the resource spans if it matches the exclude matcher
			continue
		}
		// it doesn't make sense to have both include and exclude matchers, but we support it anyway
		if p.includeMatcher != nil && !p.includeMatcher.Match(resourceSpans.Resource()) {
			// if we have an include matcher, it must match the resource for it to be processed
			continue
		}
		// it is ok that both include and exclude matchers are nil, in that case we process all spans

		// Resolve the templatization rules to use for this workload.
		// When the workload config extension is present, rules come from InstrumentationConfig;
		// workloads not configured for URL templatization are skipped entirely.
		rules, participating := p.resolveRulesForResource(resourceSpans.Resource().Attributes())
		if !participating {
			continue
		}

		for j := 0; j < resourceSpans.ScopeSpans().Len(); j++ {
			scopeSpans := resourceSpans.ScopeSpans().At(j)
			for k := 0; k < scopeSpans.Spans().Len(); k++ {
				span := scopeSpans.Spans().At(k)
				p.processSpan(span, rules)
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

func (p *urlTemplateProcessor) applyTemplatizationOnPath(path string, rules map[int][]TemplatizationRule) string {
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

	segmentRules, found := rules[len(inputPathSegments)]
	if found {
		for _, rule := range segmentRules {
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

func (p *urlTemplateProcessor) calculateTemplatedUrlFromAttr(attr pcommon.Map, rules map[int][]TemplatizationRule) (string, bool) {
	// this processor enhances url template value, which it extracts from full url or url path.
	// one of these is required for this processor to handle this span.
	urlPath, urlPathFound := getUrlPath(attr)
	if urlPathFound {
		// if url path is available, we can use it to generate the templated url
		// in case of query string, we only want the path part of the url (used with deprecated "http.target" attribute)
		templatedUrl := p.applyTemplatizationOnPath(urlPath, rules)
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
		templatedUrl := p.applyTemplatizationOnPath(parsed.Path, rules)
		return templatedUrl, true
	}

	return "", false
}

func updateHttpSpanName(span ptrace.Span, httpMethod string, templatedUrl string) {
	if templatedUrl == "" {
		return
	}
	currentName := span.Name()
	// Update when: name is exactly the method ("GET") or name is "{method} {path}" (e.g. "GET /items/1").
	// This allows templatization to normalize high-cardinality span names to "{method} {templatedPath}".
	if currentName != httpMethod && !strings.HasPrefix(currentName, httpMethod+" ") {
		return
	}
	// HTTP span names SHOULD be {method} {target} per semantic conventions (low-cardinality target).
	newSpanName := fmt.Sprintf("%s %s", httpMethod, templatedUrl)
	span.SetName(newSpanName)
}

func (p *urlTemplateProcessor) enhanceSpan(span ptrace.Span, httpMethod string, targetAttribute string, rules map[int][]TemplatizationRule) {

	attr := span.Attributes()

	templatedUrl, found := p.calculateTemplatedUrlFromAttr(attr, rules)
	if !found {
		// edge case: target attribute exists but is empty (e.g. no path) — normalize span name to method + "/"
		if val, ok := attr.Get(targetAttribute); ok && val.Type() == pcommon.ValueTypeStr && val.Str() == "" {
			updateHttpSpanName(span, httpMethod, "/")
		}
		return
	}

	// set the templated url in the target attribute and update the span name (overriding any existing value)
	attr.PutStr(targetAttribute, templatedUrl)
	updateHttpSpanName(span, httpMethod, templatedUrl)
}

func (p *urlTemplateProcessor) processSpan(span ptrace.Span, rules map[int][]TemplatizationRule) {

	attr := span.Attributes()

	httpMethod, found := getHttpMethod(attr)
	if !found {
		// we only enhance http spans, so if there is no http.method attribute, we can skip it
		return
	}

	switch span.Kind() {

	case ptrace.SpanKindClient:
		// client spans write the url templated value in "url.template" attribute.
		p.enhanceSpan(span, httpMethod, semconv.AttributeURLTemplate, rules)
	case ptrace.SpanKindServer:
		// server spans write the url templated value in "http.route" attribute.
		p.enhanceSpan(span, httpMethod, semconv.AttributeHTTPRoute, rules)
	default:
		// http spans are either client or server
		// all other spans are ignored and never enhanced
		return
	}
}
