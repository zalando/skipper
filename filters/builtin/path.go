package builtin

import (
	"regexp"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

type modPathBehavior int

const (
	regexpReplace modPathBehavior = 1 + iota
	fullReplace
)

type modPath struct {
	behavior    modPathBehavior
	rx          *regexp.Regexp
	replacement string
	template    *eskip.Template
}

// Returns a new modpath filter Spec, whose instances execute
// regexp.ReplaceAllString on the request path. Instances expect two
// parameters: the expression to match and the replacement string.
// Name: "modpath".
func NewModPath() filters.Spec { return &modPath{behavior: regexpReplace} }

// Returns a new setPath filter Spec, whose instances replace
// the request path.
//
// As an EXPERIMENTAL feature: the setPath filter provides the possiblity
// to apply template operations. The current solution supports templates
// with placeholders of the format: ${param1}, and the placeholders will
// be replaced with the values of the same name from the wildcards in the
// Path() predicate.
//
// See: https://godoc.org/github.com/zalando/skipper/routing#hdr-Wildcards
//
// The templating feature will stay in Skipper, but the syntax of the
// templating may change.
//
// See also: https://github.com/zalando/skipper/issues/182
//
// Instances expect one parameter: the new path to be set, or the path
// template to be evaluated.
//
// Name: "setPath".
func NewSetPath() filters.Spec { return &modPath{behavior: fullReplace} }

// "modPath" or "setPath"
func (spec *modPath) Name() string {
	switch spec.behavior {
	case regexpReplace:
		return ModPathName
	case fullReplace:
		return SetPathName
	default:
		panic("unspecified behavior")
	}
}

func createModPath(config []interface{}) (filters.Filter, error) {
	if len(config) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	expr, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	replacement, ok := config[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	rx, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}

	return &modPath{behavior: regexpReplace, rx: rx, replacement: replacement}, nil
}

func createSetPath(config []interface{}) (filters.Filter, error) {
	if len(config) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	tpl, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &modPath{behavior: fullReplace, template: eskip.NewTemplate(tpl)}, nil
}

// Creates instances of the modPath filter.
func (spec *modPath) CreateFilter(config []interface{}) (filters.Filter, error) {
	switch spec.behavior {
	case regexpReplace:
		return createModPath(config)
	case fullReplace:
		return createSetPath(config)
	default:
		panic("unspecified behavior")
	}
}

// Modifies the path with regexp.ReplaceAllString.
func (f *modPath) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	switch f.behavior {
	case regexpReplace:
		req.URL.Path = f.rx.ReplaceAllString(req.URL.Path, f.replacement)
	case fullReplace:
		req.URL.Path = f.template.Apply(ctx.PathParam)
	default:
		panic("unspecified behavior")
	}
}

// Noop.
func (_ *modPath) Response(_ filters.FilterContext) {}
