package normalizepath

import (
	"path"

	"github.com/zalando/skipper/filters"
)

const (
	Name = filters.NormalizePath
)

type normalizePath struct{}

func NewNormalizePath() filters.Spec { return normalizePath{} }

func (spec normalizePath) Name() string { return "normalizePath" }

func (spec normalizePath) CreateFilter(config []interface{}) (filters.Filter, error) {
	return normalizePath{}, nil
}

func (f normalizePath) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	req.URL.Path = path.Clean(req.URL.Path)
}

func (f normalizePath) Response(ctx filters.FilterContext) {}
