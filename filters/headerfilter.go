package filters

import "errors"

type headerType int

const (
	requestHeader headerType = iota
	responseHeader
)

type headerFilter struct {
	typ              headerType
	name, key, value string
}

func headerFilterConfig(config []interface{}) (string, string, error) {
	if len(config) != 2 {
		return "", "", errors.New("invalid number of args, expecting 2")
	}

	key, ok := config[0].(string)
	if !ok {
		return "", "", errors.New("invalid header key, expecting string")
	}

	value, ok := config[1].(string)
	if !ok {
		return "", "", errors.New("invalid header value, expecting string")
	}

	return key, value, nil
}

func CreateRequestHeader() Spec {
	return &headerFilter{typ: requestHeader, name: "requestHeader"}
}

func CreateResponseHeader() Spec {
	return &headerFilter{typ: responseHeader, name: "responseHeader"}
}

func (spec *headerFilter) Name() string { return spec.name }

func (spec *headerFilter) CreateFilter(config []interface{}) (Filter, error) {
	key, value, err := headerFilterConfig(config)
	return &headerFilter{typ: spec.typ, key: key, value: value}, err
}

func (f *headerFilter) Request(ctx FilterContext) {
	if f.typ == requestHeader {
		req := ctx.Request()
		if f.key == "Host" {
			req.Host = f.value
		}

		req.Header.Add(f.key, f.value)
	}
}

func (f *headerFilter) Response(ctx FilterContext) {
	if f.typ == responseHeader {
		ctx.Response().Header.Add(f.key, f.value)
	}
}
