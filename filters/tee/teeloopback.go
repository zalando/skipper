package tee

import (
	"context"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	teePredicate "github.com/zalando/skipper/predicates/tee"
	"io"
	"net/http"
	"net/url"
)

const FilterName = "teeLoopback"

type teeLoopbackSpec struct{}
type teeLoopbackFilter struct {
	teeKey string
}

func cloneRequestWithTee(req *http.Request) (*http.Request, io.ReadCloser, error) {
	u := new(url.URL)
	*u = *req.URL
	h := make(http.Header)
	for k, v := range req.Header {
		h[k] = v
	}

	var teeBody io.ReadCloser
	mainBody := req.Body

	// see proxy.go:231
	if req.ContentLength != 0 {
		pr, pw := io.Pipe()
		teeBody = pr
		mainBody = &teeTie{mainBody, pw}
	}

	clone, err := http.NewRequest(req.Method, u.String(), teeBody)
	if err != nil {
		return nil, nil, err
	}

	clone.Header = h
	clone.ContentLength = req.ContentLength

	return clone, mainBody, nil
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
	cr, body, err := cloneRequestWithTee(origRequest)
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
