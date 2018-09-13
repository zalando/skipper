package apimonitoring

import (
	"errors"
	"fmt"
	"github.com/zalando/skipper/filters"
	"regexp"
	"strings"
)

type apiMonitoringFilterSpec struct {
	Foo string // todo: Delete (was just for learning)
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

		case "ApiId":
			if len(apiId) == 0 {
				apiId = value
			} else {
				return nil, fmt.Errorf("apiId can only be specified once (is set again at index %d)", i)
			}

		case "PathPat":
			regex, err := pathPatternToRegEx(value)
			if err != nil {
				return nil, fmt.Errorf("error parsing path pattern at index %d (%q): %s", i, value, err)
			}
			pathPatterns[value] = regex

		default:
			return nil, fmt.Errorf("parameter %q at index %d is not recognized", name, i)
		}
	}

	// Create the filter
	filter = &apiMonitoringFilter{
		apiId:        apiId,
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
	RegexUrlPathPart             = `[^\/]+`
	RegexOptionalTrailingSlashes = `[\/]*`
)

// pathPatternToRegEx transforms a path pattern into a regular expression
//
// Example:		pathPattern = /orders/{orderId}/orderItem/{orderItemId}
//				      regex = \/orders\/[^\/]+\/orderItems\/[^\/]+[\/]*
//
func pathPatternToRegEx(pathPattern string) (regex *regexp.Regexp, err error) {
	pathParts := strings.Split(pathPattern, "/")
	for i, p := range pathParts {
		l := len(p)
		if l > 2 && p[0] == '{' && p[l-1] == '}' {
			pathParts[i] = RegexUrlPathPart
		}
	}
	rawRegEx := strings.Join(pathParts, `\/`) + RegexOptionalTrailingSlashes
	return regexp.Compile(rawRegEx)
}
