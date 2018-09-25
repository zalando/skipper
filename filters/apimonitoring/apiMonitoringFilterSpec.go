package apimonitoring

import (
	"fmt"
	"github.com/zalando/skipper/filters"
	"regexp"
	"strings"
)

type apiMonitoringFilterSpec struct {
	verbose bool
}

var _ filters.Spec = new(apiMonitoringFilterSpec)

const (
	ParamApiId   = "ApiId"
	ParamPathPat = "PathPat"
)

func (s *apiMonitoringFilterSpec) Name() string {
	return name
}

func (s *apiMonitoringFilterSpec) CreateFilter(args []interface{}) (filter filters.Filter, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	//
	// Parse Parameters
	//

	// initial values
	apiId := ""
	pathPatterns := make(map[string]*regexp.Regexp)

	// parse dynamic parameters
	for i, raw := range args {
		name, value, err := splitRawArg(raw)
		if err != nil {
			return nil, fmt.Errorf("error parsing parameter at index %d: %s", i, err)
		}
		switch name {

		case ParamApiId:
			if len(apiId) == 0 {
				apiId = value
			} else {
				return nil, fmt.Errorf("%q` can only be specified once (is set again at index %d)", ParamApiId, i)
			}

		case ParamPathPat:
			if err := addPathPattern(pathPatterns, value); err != nil {
				return nil, fmt.Errorf("error parsing %q at index %d (%q): %s", ParamPathPat, i, value, err)
			}

		default:
			return nil, fmt.Errorf("parameter %q at index %d is not recognized", name, i)
		}
	}

	// Create the filter
	filter = &apiMonitoringFilter{
		verbose:      s.verbose,
		apiId:        apiId,
		pathPatterns: pathPatterns,
	}
	if s.verbose {
		log.Infof("Created filter: %+v", filter)
	}
	return
}

// splitRawArg takes the raw parameter and determine its key and value
//
// Example:		raw = "pathpat: /foo/{id}"
//
//				yields	name =  "pathpat"
//						value = "/foo/{id}"
// Fails when:
//   - raw is not a string
//   - name is empty
//   - value is empty
//
func splitRawArg(raw interface{}) (name string, value string, err error) {
	rawString, ok := raw.(string)
	if !ok {
		err = fmt.Errorf("expecting string parameters, received %#v", raw)
		return
	}
	parts := strings.SplitN(rawString, ":", 2)
	if len(parts) < 2 {
		err = fmt.Errorf("expecting ':' to split the name from the value: %s", rawString)
		return
	}
	name = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	if len(name) == 0 {
		err = fmt.Errorf("parameter with no name (starts with splitter ':'): %s", rawString)
		return
	}
	if len(value) == 0 {
		err = fmt.Errorf("parameter %q does not have any value: %s", name, rawString)
		return
	}
	return
}

const (
	RegexUrlPathPart     = `[^\/]+`
	RegexOptionalSlashes = `[\/]*`
)

// addPathPattern transforms a path pattern into a regular expression and adds it to
// the provided `pathPatterns` map.
//
// Example:		 newPattern = /orders/{order-id}/order-items/{order-item-id}
//				      regex = ^\/orders\/[^\/]+\/order-items\/[^\/]+[\/]*$
//
func addPathPattern(pathPatterns map[string]*regexp.Regexp, newPattern string) error {
	normalizedPattern := strings.Trim(newPattern, "/")
	if _, ok := pathPatterns[normalizedPattern]; ok {
		return fmt.Errorf("pattern already registed: %q (normalized from %q)", newPattern, normalizedPattern)
	}

	pathParts := strings.Split(normalizedPattern, "/")
	for i, p := range pathParts {
		l := len(p)
		if l > 2 && p[0] == '{' && p[l-1] == '}' {
			pathParts[i] = RegexUrlPathPart
		}
	}
	rawRegEx := &strings.Builder{}
	rawRegEx.WriteString("^")
	rawRegEx.WriteString(RegexOptionalSlashes)
	rawRegEx.WriteString(strings.Join(pathParts, `\/`))
	rawRegEx.WriteString(RegexOptionalSlashes)
	rawRegEx.WriteString("$")

	regex, err := regexp.Compile(rawRegEx.String())
	if err != nil {
		return err
	}
	pathPatterns[normalizedPattern] = regex
	return nil
}
