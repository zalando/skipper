package apiusagemonitoring

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"regexp"
	"strings"
)

const (
	Name = "apiUsageMonitoring"

	unknownElementPlaceholder = "<unknown>"

	regexUrlPathPart     = `[^\/]+`
	regexOptionalSlashes = `[\/]*`
)

var (
	log                         = logrus.WithField("filter", Name)
	regexVarPathPartCurlyBraces = regexp.MustCompile("^{([^:{}]+)}$")
	regexVarPathPartColon       = regexp.MustCompile("^:([^:{}]+)$")
)

// NewApiUsageMonitoring creates a new instance of the API Monitoring filter
// specification (its factory).
func NewApiUsageMonitoring(enabled bool) filters.Spec {
	if !enabled {
		log.Debugf("Filter %q is not enabled. Spec returns `noop` filters.", Name)
		return &noopSpec{&noopFilter{}}
	}
	spec := &apiUsageMonitoringSpec{}
	log.Debugf("Created filter spec: %+v", spec)
	return spec
}

type apiUsageMonitoringSpec struct{}

func (s *apiUsageMonitoringSpec) Name() string {
	return Name
}

func (s *apiUsageMonitoringSpec) CreateFilter(args []interface{}) (filter filters.Filter, err error) {
	apis := parseJsonConfiguration(args)
	paths := buildPathInfoListFromConfiguration(apis)
	filter = &apiUsageMonitoringFilter{Paths: paths}
	log.Debugf("Created filter: %s", filter)
	return
}

func parseJsonConfiguration(args []interface{}) []*apiConfig {
	apis := make([]*apiConfig, 0, len(args))
	for i, a := range args {
		rawJsonConfiguration, ok := a.(string)
		if !ok {
			log.Errorf("args[%d] ignored: expecting a string, was %t", i, a)
			continue
		}
		config := new(apiConfig)
		decoder := json.NewDecoder(strings.NewReader(rawJsonConfiguration))
		decoder.DisallowUnknownFields()
		err := decoder.Decode(config)
		if err != nil {
			log.Errorf("args[%d] ignored: error reading JSON configuration: %s", i, err)
			continue
		}
		apis = append(apis, config)
	}
	return apis
}

func buildPathInfoListFromConfiguration(apis []*apiConfig) []*pathInfo {
	paths := make([]*pathInfo, 0)
	existingPathTemplates := make(map[string]*pathInfo)
	existingRegEx := make(map[string]*pathInfo)

	for apiIndex, api := range apis {

		if api.PathTemplates == nil || len(api.PathTemplates) == 0 {
			log.Errorf(
				"args[%d] ignored: does not specify any path template",
				apiIndex)
			continue
		}

		applicationId := api.ApplicationId
		if applicationId == "" {
			log.Errorf(
				"args[%d] does not specify an application ID, defaulting to %q",
				apiIndex, unknownElementPlaceholder)
			applicationId = unknownElementPlaceholder
		}

		apiId := api.ApiId
		if apiId == "" {
			log.Errorf(
				"args[%d] does not specify an API ID, defaulting to %q",
				apiIndex, unknownElementPlaceholder)
			apiId = unknownElementPlaceholder
		}

		for templateIndex, template := range api.PathTemplates {

			// Path Template validation
			if template == "" {
				log.Errorf(
					"args[%d].path_templates[%d] ignored: empty",
					apiIndex, templateIndex)
				continue
			}

			// Normalize path template and get regular expression from it
			normalisedPathTemplate, regExStr := generateRegExpStringForPathTemplate(template)

			// Create new `pathInfo` with normalized PathTemplate
			info := &pathInfo{
				ApplicationId:           applicationId,
				ApiId:                   apiId,
				PathTemplate:            normalisedPathTemplate,
				metricPrefixesPerMethod: [MethodIndexLength]*metricNames{},
			}

			// Detect path template duplicates
			if _, ok := existingPathTemplates[info.PathTemplate]; ok {
				log.Errorf(
					"args[%d].path_templates[%d] ignored: duplicate path template %q",
					apiIndex, templateIndex, info.PathTemplate)
				continue
			}
			existingPathTemplates[info.PathTemplate] = info

			// Detect regular expression duplicates
			if existingMatcher, ok := existingRegEx[regExStr]; ok {
				log.Errorf(
					"args[%d].path_templates[%d] ignored: two path templates yielded the same regular expression %q (%q and %q)",
					apiIndex, templateIndex, regExStr, info.PathTemplate, existingMatcher.PathTemplate)
				continue
			}
			existingRegEx[regExStr] = info

			// Compile the regular expression
			regEx, err := regexp.Compile(regExStr)
			if err != nil {
				log.Errorf(
					"args[%d].path_templates[%d] ignored: error compiling regular expression %q for path %q: %s",
					apiIndex, templateIndex, regExStr, info.PathTemplate, err)
				continue
			}
			info.Matcher = regEx

			// Add valid entry to the results
			paths = append(paths, info)
		}
	}

	return paths
}

// generateRegExpStringForPathTemplate normalizes the given path template and
// creates a regular expression from it.
func generateRegExpStringForPathTemplate(pathTemplate string) (normalizedPathTemplate, matcher string) {
	pathParts := strings.Split(pathTemplate, "/")
	matcherPathParts := make([]string, 0, len(pathParts))
	normalizedPathTemplateParts := make([]string, 0, len(pathParts))
	for _, p := range pathParts {
		if p == "" {
			continue
		}
		placeholderName := getVarPathPartName(p)
		if placeholderName == "" {
			// this part is not a placeholder: match it exactly
			matcherPathParts = append(matcherPathParts, p)
			normalizedPathTemplateParts = append(normalizedPathTemplateParts, p)
		} else {
			// this part is a placeholder: match a wildcard for it
			matcherPathParts = append(matcherPathParts, regexUrlPathPart)
			normalizedPathTemplateParts = append(normalizedPathTemplateParts, ":"+placeholderName)
		}
	}
	rawRegEx := &strings.Builder{}
	rawRegEx.WriteString("^")
	rawRegEx.WriteString(regexOptionalSlashes)
	rawRegEx.WriteString(strings.Join(matcherPathParts, `\/`))
	rawRegEx.WriteString(regexOptionalSlashes)
	rawRegEx.WriteString("$")

	matcher = rawRegEx.String()
	normalizedPathTemplate = strings.Join(normalizedPathTemplateParts, "/")
	return
}

// getVarPathPartName detects if a path template part represents a variadic placeholder.
// Returns "" when it is not or its name when it is.
func getVarPathPartName(pathTemplatePart string) string {
	// check if it is `:id`
	matches := regexVarPathPartColon.FindStringSubmatch(pathTemplatePart)
	if len(matches) == 2 {
		return matches[1]
	}
	// check if it is `{id}`
	matches = regexVarPathPartCurlyBraces.FindStringSubmatch(pathTemplatePart)
	if len(matches) == 2 {
		return matches[1]
	}
	// it is not a placeholder
	return ""
}
