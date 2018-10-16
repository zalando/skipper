package apimonitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"regexp"
	"strings"
)

const (
	name = "apimonitoring"

	regexUrlPathPart     = `[^\/]+`
	regexOptionalSlashes = `[\/]*`
)

var (
	log                         = logrus.WithField("filter", name)
	regexVarPathPartCurlyBraces = regexp.MustCompile("^{([^:{}]+)}$")
	regexVarPathPartColon       = regexp.MustCompile("^:([^:{}]+)$")
)

// NewApiMonitoring creates a new instance of the API Monitoring filter
// specification (its factory).
func NewApiMonitoring() filters.Spec {
	spec := new(apiMonitoringFilterSpec)
	log.Debugf("Created filter spec: %+v", spec)
	return spec
}

type apiMonitoringFilterSpec struct{}

func (s *apiMonitoringFilterSpec) Name() string {
	return name
}

func (s *apiMonitoringFilterSpec) CreateFilter(args []interface{}) (filter filters.Filter, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	if err = logAndCheckArgs(args); err != nil {
		return nil, err
	}
	config, err := parseJsonConfiguration(args)
	if err != nil {
		return nil, err
	}
	paths, err := buildPathInfoListFromConfiguration(config)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("no path to monitor")
	}

	filter = &apiMonitoringFilter{
		paths: paths,
	}
	log.Debugf("Created filter: %s", filter)
	return
}

func logAndCheckArgs(args []interface{}) error {
	log.Debugf("Creating filter with %d argument(s): %v", len(args), args)
	if len(args) < 1 {
		return errors.New("expecting one parameter (JSON configuration of the filter)")
	}
	if len(args) > 1 {
		log.Warnf("Only the first parameter is considered. The others are ignored.")
	}
	return nil
}

func parseJsonConfiguration(args []interface{}) (*filterConfig, error) {
	rawJsonConfiguration, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("expecting first parameter to be a string, was %t", args[0])
	}
	config := new(filterConfig)
	err := json.Unmarshal([]byte(rawJsonConfiguration), config)
	if err != nil {
		return nil, fmt.Errorf("error reading JSON configuration: %s", err)
	}
	log.Debugf("Filter configuration: %+v", config)
	return config, nil
}

func buildPathInfoListFromConfiguration(config *filterConfig) ([]*pathInfo, error) {
	paths := make([]*pathInfo, 0, 32)
	existingPathTemplates := make(map[string]*pathInfo)
	existingRegEx := make(map[string]*pathInfo)

	if config.ApplicationId == "" {
		return nil, errors.New("no `application_id` provided")
	}

	for templateIndex, template := range config.PathTemplates {

		// Path Template validation
		if template == "" {
			return nil, fmt.Errorf("empty path at index %d", templateIndex)
		}

		// Normalize path template and get regular expression from it
		normalisedPathTemplate, regExStr := generateRegExpStringForPathTemplate(template)

		// Create new `pathInfo` with normalized PathTemplate
		info := &pathInfo{
			ApplicationId: config.ApplicationId,
			PathTemplate:  normalisedPathTemplate,
		}

		// Detect path template duplicates
		_, ok := existingPathTemplates[info.PathTemplate]
		if ok {
			log.Infof(
				"duplicate path template %q, ignoring this template",
				info.PathTemplate)
			continue
		}
		existingPathTemplates[info.PathTemplate] = info

		// Detect regular expression duplicates
		if existingMatcher, ok := existingRegEx[regExStr]; ok {
			log.Infof(
				"two path templates yielded the same regular expression %q (%q and %q) ignoring this template",
				regExStr, info.PathTemplate, existingMatcher.PathTemplate)
			continue
		}
		existingRegEx[regExStr] = info

		// Compile the regular expression
		regEx, err := regexp.Compile(regExStr)
		if err != nil {
			return nil, fmt.Errorf(
				"error compiling regular expression %q for path %q: %s",
				regExStr, info.PathTemplate, err)
		}
		info.Matcher = regEx

		// Add valid entry to the results
		paths = append(paths, info)
	}

	return paths, nil
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
