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

	unknownPlaceholder = "{unknown}"
	noMatchPlaceholder = "{no-match}"

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
	realmKeys string,
	clientKeys string,
	realmsTrackingPattern string,
) filters.Spec {
	if !enabled {
		log.Debugf("filter %q is not enabled. spec returns `noop` filters.", Name)
		return &noopSpec{&noopFilter{}}
	}

	// parse realm keys comma separated list
	var realmKeyList []string
	for _, key := range strings.Split(realmKeys, ",") {
		strippedKey := strings.TrimSpace(key)
		if strippedKey != "" {
			realmKeyList = append(realmKeyList, strippedKey)
		}
	}
	// parse client keys comma separated list
	var clientKeyList []string
	for _, key := range strings.Split(clientKeys, ",") {
		strippedKey := strings.TrimSpace(key)
		if strippedKey != "" {
			clientKeyList = append(clientKeyList, strippedKey)
		}
	}
	// compile realms regex
	realmsTrackingMatcher, err := regexp.Compile(realmsTrackingPattern)
	if err != nil {
		log.Errorf(
			"api-usage-monitoring-realmsTrackingPattern-tracking-pattern (global config) ignored: error compiling regular expression %q: %v",
			realmsTrackingPattern, err)
		realmsTrackingMatcher = regexp.MustCompile("services")
		log.Warn("defaulting to 'services' as api-usage-monitoring-realmsTrackingPattern-tracking-pattern (global config)")
	}

	// Create the filter Spec
	var unknownPathClientTracking *clientTrackingInfo = nil // client metrics feature is disabled
	if realmKeys != "" {
		unknownPathClientTracking = &clientTrackingInfo{
			ClientTrackingMatcher: nil, // do not match anything (track `realm.{unknown}`)
			RealmsTrackingMatcher: realmsTrackingMatcher,
		}
	}
	unknownPath := newPathInfo(
		unknownPlaceholder,
		unknownPlaceholder,
		noMatchPlaceholder,
		noMatchPlaceholder,
		unknownPathClientTracking,
	)

	spec := &apiUsageMonitoringSpec{
		realmKeys:             realmKeyList,
		clientKeys:            clientKeyList,
		unknownPath:           unknownPath,
		realmsTrackingMatcher: realmsTrackingMatcher,
	}
	log.Debugf("created filter spec: %+v", spec)
	return spec
}

// apiConfig is the structure used to parse the parameters of the filter.
type apiConfig struct {
	ApplicationId         string   `json:"application_id"`
	ApiId                 string   `json:"api_id"`
	PathTemplates         []string `json:"path_templates"`
	ClientTrackingPattern string   `json:"client_tracking_pattern"`
}

type apiUsageMonitoringSpec struct {
	realmKeys             []string
	clientKeys            []string
	realmsTrackingMatcher *regexp.Regexp
	unknownPath           *pathInfo
}

func (s *apiUsageMonitoringSpec) Name() string {
	return Name
}

func (s *apiUsageMonitoringSpec) CreateFilter(args []interface{}) (filter filters.Filter, err error) {
	apis := s.parseJsonConfiguration(args)
	paths := s.buildPathInfoListFromConfiguration(apis)

	if len(paths) == 0 {
		log.Error("no valid configurations, creating noop api usage monitoring filter")
		return noopFilter{}, nil
	}

	filter = &apiUsageMonitoringFilter{
		Spec:        s,
		Paths:       paths,
		UnknownPath: s.buildUnknownPathInfo(paths),
	}
	return
}

func (s *apiUsageMonitoringSpec) parseJsonConfiguration(args []interface{}) []*apiConfig {
	apis := make([]*apiConfig, 0, len(args))
	for i, a := range args {
		rawJsonConfiguration, ok := a.(string)
		if !ok {
			log.Errorf("args[%d] ignored: expecting a string, was %t", i, a)
			continue
		}
		config := &apiConfig{
			ClientTrackingPattern: ".*", // track all clients per default
		}
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

func (s *apiUsageMonitoringSpec) buildUnknownPathInfo(paths []*pathInfo) *pathInfo {
	var applicationId *string
	for _, path := range paths {
		if applicationId != nil && *applicationId != path.ApplicationId {
			return s.unknownPath
		}
		applicationId = &path.ApplicationId
	}

	if applicationId != nil && *applicationId != "" {
		return newPathInfo(
			*applicationId,
			s.unknownPath.ApiId,
			s.unknownPath.PathLabel,
			s.unknownPath.PathTemplate,
			s.unknownPath.ClientTracking)
	}
	return s.unknownPath
}

func (s *apiUsageMonitoringSpec) buildPathInfoListFromConfiguration(apis []*apiConfig) []*pathInfo {
	var paths []*pathInfo
	existingPathTemplates := make(map[string]*pathInfo)
	existingRegEx := make(map[string]*pathInfo)

	for apiIndex, api := range apis {

		applicationId := api.ApplicationId
		if applicationId == "" {
			log.Errorf("args[%d] ignored: does not specify an application_id", apiIndex)
			continue
		}

		apiId := api.ApiId
		if apiId == "" {
			log.Errorf("args[%d] ignored: does not specify an api_id", apiIndex)
			continue
		}

		if api.PathTemplates == nil || len(api.PathTemplates) == 0 {
			log.Errorf("args[%d] ignored: does not specify any path template", apiIndex)
			continue
		}

		clientTrackingInfo := s.buildClientTrackingInfo(apiIndex, api, s.realmsTrackingMatcher)

		for templateIndex, template := range api.PathTemplates {

			// Path Template validation
			if template == "" {
				log.Errorf(
					"args[%d].path_templates[%d] ignored: empty",
					apiIndex, templateIndex)
				continue
			}

			// Normalize path template and get regular expression from it
			normalisedPathTemplate, regExStr, pathLabel := generateRegExpStringForPathTemplate(template)

			// Create new `pathInfo` with normalized PathTemplate
			info := newPathInfo(applicationId, apiId, normalisedPathTemplate, pathLabel, clientTrackingInfo)

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

func (s *apiUsageMonitoringSpec) buildClientTrackingInfo(apiIndex int, api *apiConfig, realmsTrackingMatcher *regexp.Regexp) *clientTrackingInfo {
	if len(s.realmKeys) == 0 {
		log.Infof(
			`args[%d]: skipper wide configuration "api-usage-monitoring-realm-keys" not provided, not tracking client metrics`,
			apiIndex)
		return nil
	}
	if len(s.clientKeys) == 0 {
		log.Infof(
			`args[%d]: skipper wide configuration "api-usage-monitoring-client-keys" not provided, not tracking client metrics`,
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
		RealmsTrackingMatcher: realmsTrackingMatcher,
	}
}

// generateRegExpStringForPathTemplate normalizes the given path template and
// creates a regular expression from it.
func generateRegExpStringForPathTemplate(pathTemplate string) (normalizedPathTemplate, matcher, pathLabel string) {
	pathParts := strings.Split(pathTemplate, "/")
	matcherPathParts := make([]string, 0, len(pathParts))
	normalizedPathTemplateParts := make([]string, 0, len(pathParts))
	pathLabelParts := make([]string, 0, len(pathParts))
	for _, p := range pathParts {
		if p == "" {
			continue
		}
		placeholderName := getVarPathPartName(p)
		if placeholderName == "" {
			// this part is not a placeholder: match it exactly
			matcherPathParts = append(matcherPathParts, p)
			normalizedPathTemplateParts = append(normalizedPathTemplateParts, p)
			pathLabelParts = append(pathLabelParts, p)

		} else {
			// this part is a placeholder: match a wildcard for it
			matcherPathParts = append(matcherPathParts, regexUrlPathPart)
			normalizedPathTemplateParts = append(normalizedPathTemplateParts, ":"+placeholderName)
			pathLabelParts = append(pathLabelParts, "{"+placeholderName+"}")
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
	pathLabel = strings.Join(pathLabelParts, "/")

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
