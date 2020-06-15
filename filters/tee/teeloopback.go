package tee

import (
	"context"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	teePredicate "github.com/zalando/skipper/predicates/tee"
	"net/url"
)

const FilterName = "teeLoopback"

type teeLoopbackSpec struct{}
type teeLoopbackFilter struct {
	teeKey string
}

func (t *teeLoopbackSpec) Name() string {
	return FilterName
}

func (t *teeLoopbackSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	teeKey, ok := args[0].(string)
	if !ok || teeKey == "" {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &teeLoopbackFilter{
		teeKey,
	}, nil
}

func NewTeeLoopback() filters.Spec {
	return &teeLoopbackSpec{}
}

func (f *teeLoopbackFilter) Request(ctx filters.FilterContext) {
	origRequest := ctx.Request()
	// prevent the loopback to be executed indefinitely
	teeRegistry, registryExists := ctx.Request().Context().Value(teePredicate.ContextTeeKey).(map[string]bool)
	if !registryExists {
		teeRegistry = map[string]bool{}
	}
	if _, ok := teeRegistry[f.teeKey]; ok {
		return
	}
	u := new(url.URL)
	*u = *origRequest.URL
	cr, body, err := cloneRequest(u, origRequest)
	if err != nil {
		log.Error("teeloopback: failed to clone request")
		return
	}
	origRequest.Body = body
	teeRegistry[f.teeKey] = true
	newReqContext := context.WithValue(cr.Context(), teePredicate.ContextTeeKey, teeRegistry)
	newReqWithContext := cr.WithContext(newReqContext)
	cc, _ := ctx.SplitWithRequest(newReqWithContext)
	go cc.Loopback()

}

func (f *teeLoopbackFilter) Response(_ filters.FilterContext) {}
