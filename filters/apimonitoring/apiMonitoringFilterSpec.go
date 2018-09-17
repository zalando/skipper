package apimonitoring

import (
	"errors"
	"fmt"
	"github.com/zalando/skipper/filters"
	"regexp"
	"strconv"
	"strings"
)

type apiMonitoringFilterSpec struct{}

var _ filters.Spec = new(apiMonitoringFilterSpec)

const (
	ParamApiId       = "ApiId"
	ParamPathPat     = "PathPat"
	ParamIncludePath = "IncludePath"
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
	includePath := false
	includePathIsSet := false
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

		case ParamIncludePath:
			if includePathIsSet {
				return nil, fmt.Errorf("%q can only be specified once (is set again at index %d)", ParamIncludePath, i)
			}
			includePathIsSet = true
			includePath, err = strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("error parsing %q parameter at index %d (%q): %s", ParamIncludePath, i, value, err)
			}

		default:
			return nil, fmt.Errorf("parameter %q at index %d is not recognized", name, i)
		}
	}

	// Create the filter
	filter = &apiMonitoringFilter{
		apiId:        apiId,
		includePath:  includePath,
		pathPatterns: pathPatterns,
	}
	log.Infof("Created filter: %+v", filter)
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
	if len(rawString) == 0 {
		err = errors.New("expecting non empty string")
		return
	}
	indexOfSplitter := strings.Index(rawString, ":")
	if indexOfSplitter < 0 {
		err = fmt.Errorf("expecting ':' to split the name from the value: %s", rawString)
		return
	}
	if indexOfSplitter == 0 {
		err = fmt.Errorf("parameter with no name (starts with splitter ':'): %s", rawString)
		return
	}

	name = rawString[:indexOfSplitter]
	value = strings.TrimSpace(rawString[indexOfSplitter+1:])
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
// Example:		 newPattern = /orders/{orderId}/orderItem/{orderItemId}
//				      regex = \/orders\/[^\/]+\/orderItems\/[^\/]+[\/]*
//
func addPathPattern(pathPatterns map[string]*regexp.Regexp, newPattern string) error {
	newPattern = strings.Trim(newPattern, "/")
	pathParts := strings.Split(newPattern, "/")
	for i, p := range pathParts {
		l := len(p)
		if l > 2 && p[0] == '{' && p[l-1] == '}' {
			pathParts[i] = RegexUrlPathPart
		}
	}
	rawRegEx := RegexOptionalSlashes + strings.Join(pathParts, `\/`) + RegexOptionalSlashes
	regex, err := regexp.Compile(rawRegEx)
	if err != nil {
		return err
	}
	pathPatterns[newPattern] = regex
	return nil
}
