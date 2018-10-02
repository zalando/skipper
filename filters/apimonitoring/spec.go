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

	RegexUrlPathPart     = `[^\/]+`
	RegexOptionalSlashes = `[\/]*`
)

var (
	log = logrus.WithField("filter", name)
	regexVarPathPartCurlyBraces = regexp.MustCompile("^{([^:{}]+)}$")
	regexVarPathPartColon       = regexp.MustCompile("^:([^:{}]+)$")
)

type apiMonitoringFilterSpec struct {
	verbose bool
}

var _ filters.Spec = new(apiMonitoringFilterSpec)

func (s *apiMonitoringFilterSpec) Name() string {
	return name
}

func (s *apiMonitoringFilterSpec) CreateFilter(args []interface{}) (filter filters.Filter, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	if err = logAndCheckArgs(args, s.verbose); err != nil {
		return nil, err
	}
	config, err := parseJsonConfiguration(args, s.verbose)
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

	verbose := config.Verbose
	if s.verbose && !verbose {
		log.Info("Forcing filter verbosity (global filter configuration is set to verbose)")
	}

	// Create the filter
	filter = &apiMonitoringFilter{
		verbose: verbose,
		paths:   paths,
	}
	if verbose {
		log.Infof("Created filter: %+v", filter)
	}
	return
}

func logAndCheckArgs(args []interface{}, verbose bool) error {
	if verbose {
		log.Infof("Creating filter with %d argument(s)", len(args))
		for i, v := range args {
			log.Infof("  args[%d] %#v", i, v)
		}
	}
	if len(args) < 1 {
		return errors.New("expecting one parameter (JSON configuration of the filter)")
	}
	if len(args) > 1 {
		log.Warnf("Only the first parameter is considered. The others are ignored.")
	}
	return nil
}

func parseJsonConfiguration(args []interface{}, verbose bool) (*filterConfig, error) {
	rawJsonConfiguration, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("expecting first parameter to be a string, was %t", args[0])
	}
	config := new(filterConfig)
	err := json.Unmarshal([]byte(rawJsonConfiguration), config)
	if err != nil {
		return nil, fmt.Errorf("error reading JSON configuration: %s", err)
	}
	if verbose {
		log.Infof("Filter configuration: %+v", config)
	}
	return config, nil
}

func buildPathInfoListFromConfiguration(config *filterConfig) ([]*pathInfo, error) {
	paths := make([]*pathInfo, 0, 32)
	existingPathPatterns := make(map[string]*pathInfo)
	existingRegExps := make(map[string]*pathInfo)

	for apiIndex, apiValue := range config.Apis {

		// API validation
		if apiValue.Id == "" {
			return nil, fmt.Errorf("API at index %d has no (or empty) `id`", apiIndex)
		}
		if apiValue.ApplicationId == "" {
			return nil, fmt.Errorf("API at index %d has no (or empty) `application_id`", apiIndex)
		}

		for templateIndex, template := range apiValue.PathTemplates {

			// Path Pattern validation
			if template == "" {
				return nil, fmt.Errorf("API at index %d has empty path at index %d", apiIndex, templateIndex)
			}

			// Normalize path template and get regular expression from it
			normalisedPathTemplate, regExStr := generateRegExpStringForPathPattern(template)

			// Create new `pathInfo` with normalized PathTemplate
			info := &pathInfo{
				ApiId:         apiValue.Id,
				ApplicationId: apiValue.ApplicationId,
				PathTemplate:  normalisedPathTemplate,
			}

			// Detect path pattern duplicates
			if existingPathPattern, ok := existingPathPatterns[info.PathTemplate]; ok {
				return nil, fmt.Errorf(
					"duplicate path pattern %q detected: %+v and %+v",
					info.PathTemplate, existingPathPattern, info)
			}
			existingPathPatterns[info.PathTemplate] = info

			// Generate the regular expression for this path pattern and detect duplicates
			if existingRegEx, ok := existingRegExps[regExStr]; ok {
				return nil, fmt.Errorf(
					"two path patterns yielded the same regular expression %q: %+v and %+v",
					regExStr, existingRegEx, info)
			}
			existingRegExps[regExStr] = info

			// Compile the regular expression
			regEx, err := regexp.Compile(regExStr)
			if err != nil {
				return nil, fmt.Errorf(
					"error compiling regular expression %s for path %+v: %s",
					regExStr, info, err)
			}
			info.Matcher = regEx

			// Add valid entry to the results
			paths = append(paths, info)
		}
	}

	return paths, nil
}

// generateRegExpForPathPattern creates a regular expression from a path pattern.
//
// Example:    pathTemplate = /orders/{order-id}/order-items/{order-item-id}
//				      regex = ^\/orders\/[^\/]+\/order-items\/[^\/]+[\/]*$
//
func generateRegExpStringForPathPattern(pathTemplate string) (normalizedPathTemplate, matcher string) {
	pathParts := strings.Split(pathTemplate, "/")
	matcherPathParts := make([]string, 0, len(pathParts))
	normalizedPathTemplateParts := make([]string, 0, len(pathParts))
	for _, p := range pathParts {
		if p == "" {
			continue
		}
		placeholderName := getVarPathPartName(p)
		if placeholderName != "" {
			matcherPathParts = append(matcherPathParts, RegexUrlPathPart)
			normalizedPathTemplateParts = append(normalizedPathTemplateParts, ":" + placeholderName)
		} else {
			matcherPathParts = append(matcherPathParts, p)
			normalizedPathTemplateParts = append(normalizedPathTemplateParts, p)
		}
	}
	rawRegEx := &strings.Builder{}
	rawRegEx.WriteString("^")
	rawRegEx.WriteString(RegexOptionalSlashes)
	rawRegEx.WriteString(strings.Join(matcherPathParts, `\/`))
	rawRegEx.WriteString(RegexOptionalSlashes)
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
