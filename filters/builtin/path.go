package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/helpers"
	"regexp"
)

type modPathBehavior int

const (
	regexpReplace modPathBehavior = 1 + iota
	fullReplace
	transformReplace
)

type modPath struct {
	behavior    modPathBehavior
	rx          *regexp.Regexp
	replacement string
	template    *helpers.TemplateString
}

// Returns a new modpath filter Spec, whose instances execute
// regexp.ReplaceAllString on the request path. Instances expect two
// parameters: the expression to match and the replacement string.
// Name: "modpath".
func NewModPath() filters.Spec { return &modPath{behavior: regexpReplace} }

// Returns a new setPath filter Spec, whose instances replace
// the request path. Instances expect one parameter: the new path
// to be set.
// Name: "setPath".
func NewSetPath() filters.Spec { return &modPath{behavior: fullReplace} }

// Returns a new transformPath filter Spec, whose instances builds
// the request path replacing the params.
// Instances expect one parameter: the new path to be transformed to.
// Name: "transformPath".
func NewTransformPath() filters.Spec { return &modPath{behavior: transformReplace} }

// "modPath" or "setPath"
func (spec *modPath) Name() string {
	switch spec.behavior {
	case regexpReplace:
		return ModPathName
	case fullReplace:
		return SetPathName
	case transformReplace:
		return TransformPathName
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

	replacement, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &modPath{behavior: fullReplace, replacement: replacement}, nil
}

func createTransformPath(config []interface{}) (filters.Filter, error) {
	if len(config) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	template, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &modPath{behavior: transformReplace, template: helpers.NewTemplateString(template)}, nil
}

// Creates instances of the modPath filter.
func (spec *modPath) CreateFilter(config []interface{}) (filters.Filter, error) {
	switch spec.behavior {
	case regexpReplace:
		return createModPath(config)
	case fullReplace:
		return createSetPath(config)
	case transformReplace:
		return createTransformPath(config)
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
		req.URL.Path = f.replacement
	case transformReplace:
		req.URL.Path = f.template.ApplyWithGetter(ctx.PathParam)
	default:
		panic("unspecified behavior")
	}
}

// Noop.
func (_ *modPath) Response(_ filters.FilterContext) {}
