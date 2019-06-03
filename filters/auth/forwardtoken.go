package auth

import (
	"encoding/json"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"golang.org/x/net/http/httpguts"
)

const (
	ForwardTokenName = "forwardToken"
)

type (
	forwardTokenSpec struct {
	}
	forwardTokenFilter struct {
		HeaderName     string
		RetainJsonKeys []string
	}
)

// NewForwardToken creates a filter to forward the result of token info or
// token introspection to the backend server.
func NewForwardToken() filters.Spec {
	return &forwardTokenSpec{}
}
func (s *forwardTokenSpec) Name() string {
	return ForwardTokenName
}

func (*forwardTokenSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	headerName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	valid := httpguts.ValidHeaderFieldName(headerName)
	if !valid {
		return nil, fmt.Errorf("header name %s in invalid", headerName)
	}

	remainingArgs := args[1:]
	stringifiedRemainingArgs := make([]string, len(remainingArgs))
	for i, v := range remainingArgs {
		maskedKeyName, ok := v.(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		stringifiedRemainingArgs[i] = maskedKeyName
	}

	return &forwardTokenFilter{HeaderName: headerName, RetainJsonKeys: stringifiedRemainingArgs}, nil
}

func getTokenPayload(ctx filters.FilterContext, cacheKey string) interface{} {
	cachedValue, ok := ctx.StateBag()[cacheKey]
	if !ok {
		return nil
	}
	return cachedValue
}

func (f *forwardTokenFilter) Request(ctx filters.FilterContext) {
	tiMap := getTokenPayload(ctx, tokeninfoCacheKey)
	if tiMap == nil {
		tiMap = getTokenPayload(ctx, tokenintrospectionCacheKey)
	}
	if tiMap == nil {
		return
	}

	if len(f.RetainJsonKeys) > 0 {
		switch typedTiMap := tiMap.(type) {
		case map[string]interface{}:
			tiMap = retainKeys(typedTiMap, f.RetainJsonKeys)
		case tokenIntrospectionInfo:
			tiMap = retainKeys(typedTiMap, f.RetainJsonKeys)
		default:
			log.Errorf("Unexpected input type[%T] for `forwardToken` filter. Unable to apply mask", typedTiMap)
		}
	}

	payload, err := json.Marshal(tiMap)
	if err != nil {
		return
	}
	request := ctx.Request()
	jsonHeader := strings.TrimSpace(string(payload))
	request.Header.Add(f.HeaderName, jsonHeader)
}

func (f *forwardTokenFilter) Response(filters.FilterContext) {
}

func retainKeys(data map[string]interface{}, keys []string) map[string]interface{} {
	whitelistedKeys := make(map[string]interface{})
	for _, v := range keys {
		if val, ok := data[v]; ok {
			whitelistedKeys[v] = val
		}
	}

	return whitelistedKeys
}
