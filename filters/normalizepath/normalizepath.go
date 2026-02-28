package normalizepath

import (
	"strings"

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

	segments := strings.Split(req.URL.Path, "/")
	var filteredSegments []string
	for _, seg := range segments {
		if seg != "" {
			filteredSegments = append(filteredSegments, seg)
		}
	}
	normalizedPath := "/" + strings.Join(filteredSegments, "/")

	req.URL.Path = normalizedPath
}

func (f normalizePath) Response(ctx filters.FilterContext) {}
