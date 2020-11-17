package tracing

import (
	"fmt"

	"github.com/opentracing/opentracing-go"

	"github.com/zalando/skipper/filters"
	logfilter "github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/oauth"
)

const (
	StateBagToTagFilterName = "stateBagToTag"
)

type stateBagToTagSpec struct {
	maskUser []oauth.MaskOAuthUser
}

type stateBagToTagFilter struct {
	stateBagItemName string
	tagName          string
	maskUser         []oauth.MaskOAuthUser
}

func (stateBagToTagSpec) Name() string {
	return StateBagToTagFilterName
}

func (s stateBagToTagSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	stateBagItemName, ok := args[0].(string)
	if !ok || stateBagItemName == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	tagName := stateBagItemName
	if len(args) > 1 {
		tagNameArg, ok := args[1].(string)
		if !ok || tagNameArg == "" {
			return nil, filters.ErrInvalidFilterParameters
		}
		tagName = tagNameArg
	}

	return stateBagToTagFilter{
		stateBagItemName: stateBagItemName,
		tagName:          tagName,
		maskUser:         s.maskUser,
	}, nil
}

func NewStateBagToTag(maskUser []oauth.MaskOAuthUser) filters.Spec {
	return stateBagToTagSpec{maskUser: maskUser}
}

func (f stateBagToTagFilter) Request(ctx filters.FilterContext) {
	span := opentracing.SpanFromContext(ctx.Request().Context())
	if span == nil {
		return
	}

	stateBag := ctx.StateBag()
	if f.stateBagItemName == logfilter.AuthUserKey {
		for _, user := range f.maskUser {
			if replacement, ok := user(stateBag); ok {
				span.SetTag(f.tagName, replacement)
				return
			}
		}
	}

	value, ok := stateBag[f.stateBagItemName]
	if !ok {
		return
	}
	span.SetTag(f.tagName, fmt.Sprint(value))
}

func (stateBagToTagFilter) Response(ctx filters.FilterContext) {}
