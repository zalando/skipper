package apiusagemonitoring

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/zalando/skipper/filters"
)

const (
	ApiUsageMonitoringName = "apiUsageMonitoring"

	unknownPlaceholder = "{unknown}"
	noMatchPlaceholder = "{no-match}"
	noTagPlaceholder   = "{no-tag}"
)

var (
	log      = logrus.WithField("filter", ApiUsageMonitoringName)
	regCache = sync.Map{}
)

func loadOrCompileRegex(pattern string) (*regexp.Regexp, error) {
	var err error
	var reg *regexp.Regexp
	regI, ok := regCache.Load(pattern)
	if !ok {
		reg, err = regexp.Compile(pattern)
		regCache.Store(pattern, reg)
	} else {
		reg = regI.(*regexp.Regexp)
	}
	return reg, err
}

// NewApiUsageMonitoring creates a new instance of the API Monitoring filter
// specification (its factory).
func NewApiUsageMonitoring(
	enabled bool,
	realmKeys string,
	clientKeys string,
	realmsTrackingPattern string,
) filters.Spec {
	if !enabled {
		log.Debugf("filter %q is not enabled. spec returns `noop` filters.", ApiUsageMonitoringName)
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

	realmsTrackingMatcher, err := loadOrCompileRegex(realmsTrackingPattern)
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
		noTagPlaceholder,
		unknownPlaceholder,
		noMatchPlaceholder,
		unknownPathClientTracking,
	)

	spec := &apiUsageMonitoringSpec{
		pathHandler:           defaultPathHandler{},
		realmKeys:             realmKeyList,
		clientKeys:            clientKeyList,
		unknownPath:           unknownPath,
		realmsTrackingMatcher: realmsTrackingMatcher,
	}
	log.Debugf("created filter spec: %+v", spec)
	return spec
}

// ApiConfig is the structure used to parse the parameters of the filter.
type ApiConfig struct {
	ApplicationId         string   `json:"application_id"`
	Tag                   string   `json:"tag"`
	ApiId                 string   `json:"api_id"`
	PathTemplates         []string `json:"path_templates"`
	ClientTrackingPattern string   `json:"client_tracking_pattern"`
}

type apiUsageMonitoringSpec struct {
	pathHandler           pathHandler
	realmKeys             []string
	clientKeys            []string
	realmsTrackingMatcher *regexp.Regexp
	unknownPath           *pathInfo
}

func (s *apiUsageMonitoringSpec) Name() string {
	return ApiUsageMonitoringName
}

func (s *apiUsageMonitoringSpec) CreateFilter(args []interface{}) (filter filters.Filter, err error) {
	apis := s.parseJsonConfiguration(args)
	paths := s.buildPathInfoListFromConfiguration(apis)

	if len(paths) == 0 {
		log.Error("no valid configurations, creating noop api usage monitoring filter")
		return noopFilter{}, nil
	}

	filter = &apiUsageMonitoringFilter{
		realmKeys:   s.realmKeys,
		clientKeys:  s.clientKeys,
		Paths:       paths,
		UnknownPath: s.buildUnknownPathInfo(paths),
	}
	return
}

func (s *apiUsageMonitoringSpec) parseJsonConfiguration(args []interface{}) []*ApiConfig {
	apis := make([]*ApiConfig, 0, len(args))
	for i, a := range args {
		rawJsonConfiguration, ok := a.(string)
		if !ok {
			log.Errorf("args[%d] ignored: expecting a string, was %t", i, a)
			continue
		}
		config := &ApiConfig{
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
			s.unknownPath.Tag,
			s.unknownPath.ApiId,
			s.unknownPath.PathTemplate,
			s.unknownPath.ClientTracking)
	}
	return s.unknownPath
}

func (s *apiUsageMonitoringSpec) buildPathInfoListFromConfiguration(apis []*ApiConfig) []*pathInfo {
	var paths []*pathInfo
	existingPathTemplates := make(map[string]*pathInfo)
	existingPathPattern := make(map[string]*pathInfo)

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

			// Normalize path template and get regular expression path pattern
			pathTemplate := s.pathHandler.normalizePathTemplate(template)
			pathPattern := s.pathHandler.createPathPattern(template)

			// Create new `pathInfo` with normalized PathTemplate
			info := newPathInfo(applicationId, api.Tag, apiId, pathTemplate, clientTrackingInfo)

			// Detect path template duplicates
			if _, ok := existingPathTemplates[info.PathTemplate]; ok {
				log.Errorf(
					"args[%d].path_templates[%d] ignored: duplicate path template %q",
					apiIndex, templateIndex, info.PathTemplate)
				continue
			}
			existingPathTemplates[info.PathTemplate] = info

			// Detect regular expression duplicates
			if existingMatcher, ok := existingPathPattern[pathPattern]; ok {
				log.Errorf(
					"args[%d].path_templates[%d] ignored: two path templates yielded the same regular expression %q (%q and %q)",
					apiIndex, templateIndex, pathPattern, info.PathTemplate, existingMatcher.PathTemplate)
				continue
			}
			existingPathPattern[pathPattern] = info

			pathPatternMatcher, err := loadOrCompileRegex(pathPattern)
			if err != nil {
				log.Errorf(
					"args[%d].path_templates[%d] ignored: error compiling regular expression %q for path %q: %v",
					apiIndex, templateIndex, pathPattern, info.PathTemplate, err)
				continue
			}
			if pathPatternMatcher == nil {
				continue
			}

			info.Matcher = pathPatternMatcher

			// Add valid entry to the results
			paths = append(paths, info)
		}
	}

	// Sort the paths by their matcher's RegEx
	sort.Sort(pathInfoByRegExRev(paths))

	return paths
}

func (s *apiUsageMonitoringSpec) buildClientTrackingInfo(apiIndex int, api *ApiConfig, realmsTrackingMatcher *regexp.Regexp) *clientTrackingInfo {
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

	clientTrackingMatcher, err := loadOrCompileRegex(api.ClientTrackingPattern)
	if err != nil {
		log.Errorf(
			"args[%d].client_tracking_pattern ignored (no client tracking): error compiling regular expression %q: %v",
			apiIndex, api.ClientTrackingPattern, err)
		return nil
	}
	if clientTrackingMatcher == nil {
		return nil
	}

	return &clientTrackingInfo{
		ClientTrackingMatcher: clientTrackingMatcher,
		RealmsTrackingMatcher: realmsTrackingMatcher,
	}
}

var (
	regexpMultipleSlashes   = regexp.MustCompile(`/+`)
	regexpLeadingSlashes    = regexp.MustCompile(`^/*`)
	regexpTrailingSlashes   = regexp.MustCompile(`/*$`)
	regexpMiddleSlashes     = regexp.MustCompile(`([^/^])/+([^/*])`)
	rexexpSlashColumnVar    = regexp.MustCompile(`/:([^:{}/]*)`)
	rexexpCurlyBracketVar   = regexp.MustCompile(`{([^{}]*?)([?]?)}`)
	regexpEscapeBeforeChars = regexp.MustCompile(`([.*+?\\])`)
	regexpEscapeAfterChars  = regexp.MustCompile(`([{}[\]()|])`)
)

// pathHandler path handler interface.
type pathHandler interface {
	normalizePathTemplate(path string) string
	createPathPattern(path string) string
}

// defaultPathHandler default path handler implementation.
type defaultPathHandler struct{}

// normalizePathTemplate normalize path template removing the leading and
// trailing slashes, substituting multiple adjacent slashes with a single
// one, and replacing column based variable declarations by curly bracked
// based.
func (ph defaultPathHandler) normalizePathTemplate(path string) string {
	path = regexpLeadingSlashes.ReplaceAllString(path, "")
	path = regexpTrailingSlashes.ReplaceAllString(path, "")
	path = regexpMultipleSlashes.ReplaceAllString(path, "/")
	path = rexexpSlashColumnVar.ReplaceAllString(path, "/{$1}")
	path = rexexpCurlyBracketVar.ReplaceAllString(path, "{$1}")
	return path
}

// createPathPattern create a regular expression path pattern for a path
// template by escaping regular specific characters, add optional matching
// of leading and trailing slashes, accept adjacent slashes as if a single
// slash was given, and allow free matching of content on variable locations.
func (ph defaultPathHandler) createPathPattern(path string) string {
	path = regexpEscapeBeforeChars.ReplaceAllString(path, "\\$1")
	path = rexexpSlashColumnVar.ReplaceAllString(path, "/.+")
	path = rexexpCurlyBracketVar.ReplaceAllStringFunc(path, selectPathVarPattern)
	path = regexpLeadingSlashes.ReplaceAllString(path, "^/*")
	path = regexpTrailingSlashes.ReplaceAllString(path, "/*$")
	path = regexpMiddleSlashes.ReplaceAllString(path, "$1/+$2")
	path = regexpEscapeAfterChars.ReplaceAllString(path, "\\$1")
	return path
}

// selectPathVarPattern select the correct path variable pattern depending
// on the path variable syntax. A trailing question mark is interpreted as
// a path variable that is allowed to be empty.
func selectPathVarPattern(match string) string {
	if strings.HasSuffix(match, "\\?}") {
		return ".*"
	}
	return ".+"
}
