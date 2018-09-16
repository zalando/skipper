package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zalando/skipper/filters"
	"golang.org/x/net/http/httpguts"
	"strings"
)

const (
	ForwardTokenName = "forwardtoken"
)

type (
	forwardTokenSpec struct {
	}
	forwardTokenFilter struct {
		HeaderName string
	}
)

func NewForwardToken() filters.Spec {
	return &forwardTokenSpec{}
}
func (s *forwardTokenSpec) Name() string {
	return ForwardTokenName
}

func (*forwardTokenSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	headerName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	valid := httpguts.ValidHeaderFieldName(headerName)
	if !valid {
		return nil, errors.New(fmt.Sprintf("header name %s in invalid", headerName))
	}
	return &forwardTokenFilter{HeaderName: headerName}, nil
}

func getTokenPayload(ctx filters.FilterContext, cacheKey string) map[string]interface{} {
	cachedValue, ok := ctx.StateBag()[cacheKey]
	if !ok {
		return nil
	}
	tiMap, ok := cachedValue.(map[string]interface{})
	if !ok {
		return nil
	}
	return tiMap
}

func (f *forwardTokenFilter) Request(ctx filters.FilterContext) {
	tiMap := getTokenPayload(ctx, tokeninfoCacheKey)
	if tiMap == nil {
		tiMap = getTokenPayload(ctx, tokenintrospectionCacheKey)
	}
	if tiMap == nil {
		return
	}
	jsonBuffer := new(bytes.Buffer)
	encoder := json.NewEncoder(jsonBuffer)
	err := encoder.Encode(tiMap)
	if err != nil {
		return
	}
	request := ctx.Request()
	jsonHeader := strings.TrimRight(jsonBuffer.String(), "\n")
	request.Header.Add(f.HeaderName, jsonHeader)
}

func (f *forwardTokenFilter) Response(filters.FilterContext) {
}
