package apiusagemonitoring

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"regexp"
	"sort"
	"strings"
)

const (
	Name = "apiUsageMonitoring"

	unknownElementPlaceholder = "<unknown>"

	regexUrlPathPart     = `.+`
	regexOptionalSlashes = `\/*`
)

var (
	log                         = logrus.WithField("filter", Name)
	regexVarPathPartCurlyBraces = regexp.MustCompile("^{([^:{}]+)}$")
	regexVarPathPartColon       = regexp.MustCompile("^:([^:{}]+)$")
)

// NewApiUsageMonitoring creates a new instance of the API Monitoring filter
// specification (its factory).
func NewApiUsageMonitoring(
	enabled bool,
	realmKeyName string,
	clientIdKeyName string,
) filters.Spec {
	if !enabled {
		log.Debugf("Filter %q is not enabled. Spec returns `noop` filters.", Name)
		return &noopSpec{&noopFilter{}}
	}

	// Parse client ID keys comma separated list
	clientIdKeys := make([]string, 0)
	for _, key := range strings.Split(clientIdKeyName, ",") {
		strippedKey := strings.TrimSpace(key)
		if strippedKey != "" {
			clientIdKeys = append(clientIdKeys, strippedKey)
		}
	}

	// Create the filter Spec
	var unknownPath *clientTrackingInfo = nil // client metrics feature is disabled
	if realmKeyName != "" {
		unknownPath = &clientTrackingInfo{
			ClientTrackingMatcher: nil, // do not match anything (track `realm.<unknown>`)
		}
	}
	spec := &apiUsageMonitoringSpec{
		realmKey:    realmKeyName,
		clientIdKey: clientIdKeys,
		unknownPath: &pathInfo{
			ApplicationId:           unknownElementPlaceholder,
			ApiId:                   unknownElementPlaceholder,
			PathTemplate:            unknownElementPlaceholder,
			metricPrefixesPerMethod: [MethodIndexLength]*metricNames{},
			ClientTracking:          unknownPath,
		},
	}
	log.Debugf("Created filter spec: %+v", spec)
	return spec
}

type apiUsageMonitoringSpec struct {
	realmKey    string
	clientIdKey []string
	unknownPath *pathInfo
}

func (s *apiUsageMonitoringSpec) Name() string {
	return Name
}

func (s *apiUsageMonitoringSpec) CreateFilter(args []interface{}) (filter filters.Filter, err error) {
	apis := parseJsonConfiguration(args)
	paths := s.buildPathInfoListFromConfiguration(apis)

	filter = &apiUsageMonitoringFilter{
		Spec:  s,
		Paths: paths,
	}
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

func (s *apiUsageMonitoringSpec) buildPathInfoListFromConfiguration(apis []*apiConfig) []*pathInfo {
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

		clientTrackingInfo := s.buildClientTrackingInfo(apiIndex, api)

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
				ClientTracking:          clientTrackingInfo,
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
					"args[%d].path_templates[%d] ignored: error compiling regular expression %q for path %q: %v",
					apiIndex, templateIndex, regExStr, info.PathTemplate, err)
				continue
			}
			info.Matcher = regEx

			// Add valid entry to the results
			paths = append(paths, info)
		}
	}

	// Sort the paths by their matcher's RegEx
	sort.Sort(pathInfoByRegExRev(paths))

	return paths
}

func (s *apiUsageMonitoringSpec) buildClientTrackingInfo(apiIndex int, api *apiConfig) *clientTrackingInfo {
	if s.realmKey == "" {
		log.Infof(
			`args[%d]: skipper wide configuration "api-usage-monitoring-realm-key" not provided, not tracking client metrics`,
			apiIndex)
		return nil
	}
	if len(s.clientIdKey) == 0 {
		log.Infof(
			`args[%d]: skipper wide configuration "api-usage-monitoring-client-id-key" not provided, not tracking client metrics`,
			apiIndex)
		return nil
	}
	if api.ClientTrackingPattern == "" {
		log.Debugf(
			`args[%d]: empty client_tracking_pattern disables the client metrics for its endpoints`,
			apiIndex)
		return nil
	}

	clientTrackingMatcher, err := regexp.Compile(api.ClientTrackingPattern)
	if err != nil {
		log.Errorf(
			"args[%d].client_tracking_pattern ignored (no client tracking): error compiling regular expression %q: %v",
			apiIndex, api.ClientTrackingPattern, err)
		return nil
	}

	return &clientTrackingInfo{
		ClientTrackingMatcher: clientTrackingMatcher,
	}
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
