// Package annotate provides filters that allow to add annotations to
// a route.
package annotate

import (
	"fmt"

	"github.com/zalando/skipper/filters"
)

type (
	annotateSpec struct{}

	annotateFilter struct {
		key, value string
	}
)

const annotateStateBagKey = "filter." + filters.AnnotateName

// New creates filters to annotate a filter chain.
// It stores its key and value arguments into the filter context.
// Use [GetAnnotations] to retrieve the annotations from the context.
func New() filters.Spec {
	return annotateSpec{}
}

func (annotateSpec) Name() string {
	return filters.AnnotateName
}

func (as annotateSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("requires string key and value arguments")
	}

	af, ok := &annotateFilter{}, false
	if af.key, ok = args[0].(string); !ok {
		return nil, fmt.Errorf("key argument must be a string")
	}

	if af.value, ok = args[1].(string); !ok {
		return nil, fmt.Errorf("value argument must be a string")
	}

	return af, nil
}

func (af *annotateFilter) Request(ctx filters.FilterContext) {
	if v, ok := ctx.StateBag()[annotateStateBagKey]; ok {
		v.(map[string]string)[af.key] = af.value
	} else {
		ctx.StateBag()[annotateStateBagKey] = map[string]string{af.key: af.value}
	}
}

func (af *annotateFilter) Response(filters.FilterContext) {}

func GetAnnotations(ctx filters.FilterContext) map[string]string {
	if v, ok := ctx.StateBag()[annotateStateBagKey]; ok {
		return v.(map[string]string)
	}
	return nil
}
