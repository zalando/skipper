package wasm

import (
	"context"
	"net/url"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/zalando/skipper/filters"
)

type wasmSpec struct{}

type wasm struct {
	code     []byte
	mod      api.Module
	request  api.Function
	response api.Function
}

func NewWASM() filters.Spec {
	return &wasmSpec{}
}

// Name implements filters.Spec.
func (*wasmSpec) Name() string {
	return filters.WASMName
}

// CreateFilter implements filters.Spec.
func (*wasmSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	src, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	u, err := url.Parse(src)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	var code []byte

	switch u.Scheme {
	case "file":
		code, err = os.ReadFile(u.Path)
		if err != nil {
			logrus.Errorf("Failed to load file %q: %v", u.Path, err)
			return nil, filters.ErrInvalidFilterParameters
		}
	case "https":
		panic("not implemented")
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx) // TODO: needs r.Close()
	// Instantiate WASI, which implements host functions needed for TinyGo to
	// implement `panic`.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Instantiate the guest Wasm into the same runtime. It exports the `add`
	// function, implemented in WebAssembly.
	mod, err := r.Instantiate(ctx, code)
	if err != nil {
		logrus.Fatalf("failed to instantiate module: %v", err)
	}
	request := mod.ExportedFunction("request")
	response := mod.ExportedFunction("response")

	return &wasm{
		code:     code,
		mod:      mod,
		request:  request,
		response: response,
	}, nil
}

// Request implements filters.Filter.
func (w *wasm) Request(filters.FilterContext) {

	result, err := w.request.Call(context.Background(), 2, 3)
	if err != nil {
		logrus.Errorf("failed to call add: %v", err)
	}
	logrus.Infof("request result: %v", result)

}

// Response implements filters.Filter.
func (w *wasm) Response(filters.FilterContext) {
	result, err := w.response.Call(context.Background(), 3, 2)
	if err != nil {
		logrus.Errorf("failed to call add: %v", err)
	}
	logrus.Infof("response result: %v", result)

}
