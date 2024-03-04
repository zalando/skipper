package wasm

import (
	"context"
	"fmt"
	"net/url"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/zalando/skipper/filters"
)

type WASMOpts struct {
	Typ      string
	CacheDir string
}

const (
	memoryLimitPages uint32 = 8 // 8*2^16
)

type cache int

type compilationCache cache

const (
	none cache = iota + 1
	inmemory
	filesystem
)

type wasmSpec struct {
	typ      cache
	cacheDir string
}

// TODO(sszuecs): think about:
//
// 1) If we want to provide internal Go functions to support our wasm
// modules, we can use
// https://pkg.go.dev/github.com/tetratelabs/wazero#HostFunctionBuilder,
// such that WASM binary can import and use these functions.
// see also https://pkg.go.dev/github.com/tetratelabs/wazero#HostModuleBuilder
type wasm struct {
	code     []byte
	runtime  wazero.Runtime
	mod      api.Module
	request  api.Function
	response api.Function
}

func NewWASM(o WASMOpts) filters.Spec {
	typ := none
	switch o.Typ {
	case "none":
		typ = none
	case "in-memory":
		typ = inmemory
	case "fs":
		typ = filesystem
	default:
		log.Errorf("Failed to find compilation cache type %q, available values 'none', 'in-memory' and 'fs'", typ)
	}

	return &wasmSpec{
		typ:      typ,
		cacheDir: o.CacheDir,
	}
}

// Name implements filters.Spec.
func (*wasmSpec) Name() string {
	return filters.WASMName
}

// CreateFilter implements filters.Spec.
func (ws *wasmSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
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
			return nil, fmt.Errorf("failed to load file %q: %v", u.Path, filters.ErrInvalidFilterParameters)
		}
	case "https":
		panic("not implemented")
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	ctx := context.Background()

	var r wazero.Runtime
	switch ws.typ {
	// in general, we likely do not need compilation
	// cache, but likely we want to use Pre-/PostProcessor
	// to not recreate the filter and check in
	// CreateFilter to not compile the WASM code again and
	// again
	case none:
		// we could try to use NewRuntimeConfigCompiler for
		// GOARCH specific asm for optimal performance as
		// stated in
		// https://pkg.go.dev/github.com/tetratelabs/wazero#NewRuntimeConfigCompiler
		config := wazero.NewRuntimeConfig().WithMemoryLimitPages(memoryLimitPages)
		r = wazero.NewRuntimeWithConfig(ctx, config)

	case inmemory:
		// TODO(sszuecs): unclear if we hit the bug described in https://pkg.go.dev/github.com/tetratelabs/wazero#RuntimeConfig for WithCompilationCache():

		// Cached files are keyed on the version of wazero. This is obtained from go.mod of your application,
		// and we use it to verify the compatibility of caches against the currently-running wazero.
		// However, if you use this in tests of a package not named as `main`, then wazero cannot obtain the correct
		// version of wazero due to the known issue of debug.BuildInfo function: https://github.com/golang/go/issues/33976.
		// As a consequence, your cache won't contain the correct version information and always be treated as `dev` version.
		// To avoid this issue, you can pass -ldflags "-X github.com/tetratelabs/wazero/internal/version.version=foo" when running tests.

		cache := wazero.NewCompilationCache()
		config := wazero.NewRuntimeConfig().WithCompilationCache(cache)
		config = config.WithMemoryLimitPages(memoryLimitPages)
		r = wazero.NewRuntimeWithConfig(ctx, config)

	case filesystem:
		cache, err := wazero.NewCompilationCacheWithDir(ws.cacheDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create compilation cache dir with %q as directory: %w", ws.cacheDir, filters.ErrInvalidFilterParameters)
		}

		config := wazero.NewRuntimeConfig().WithCompilationCache(cache)
		config = config.WithMemoryLimitPages(memoryLimitPages)
		r = wazero.NewRuntimeWithConfig(ctx, config)

	default:
		return nil, fmt.Errorf("failed to create wazero runtime typ %q: %w", ws.typ, filters.ErrInvalidFilterParameters)
	}

	// Instantiate WASI, which implements host functions needed for TinyGo to
	// implement `panic`.
	// see also https://github.com/tetratelabs/wazero/blob/main/imports/README.md
	// and https://wazero.io/languages/
	//
	// we do not need the closer because of https://pkg.go.dev/github.com/tetratelabs/wazero@v1.6.0/imports/wasi_snapshot_preview1#hdr-Notes
	// "Closing the wazero.Runtime has the same effect as closing the result."
	_, err = wasi_snapshot_preview1.Instantiate(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("failed to wasi_snapshot_preview1: %w: %w", err, filters.ErrInvalidFilterParameters)
	}

	// TODO(sszuecs): create modules to be used from user wasm code
	// cmod, err := r.CompileModule(ctx, []byte(""))
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to compile module: %w: %w", err, filters.ErrInvalidFilterParameters)
	// }
	// r.InstantiateModule(ctx)
	//
	// Instantiate the guest Wasm into the same runtime. It exports the `add`
	// function, implemented in WebAssembly.
	// mod, err := r.Instantiate(ctx, cmod, moduleConfig)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to instantiate module: %w: %w", err, filters.ErrInvalidFilterParameters)
	// }

	mod, err := r.Instantiate(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate module: %w: %w", err, filters.ErrInvalidFilterParameters)
	}
	request := mod.ExportedFunction("request")
	response := mod.ExportedFunction("response")

	return &wasm{
		code:     code,
		runtime:  r,
		mod:      mod,
		request:  request,
		response: response,
	}, nil
}

func (w *wasm) Request(ctx filters.FilterContext) {

	result, err := w.request.Call(ctx.Request().Context(), 2, 3)
	if err != nil {
		log.Errorf("failed to call add: %v", err)
	}
	log.Infof("request result: %v", result)

}

func (w *wasm) Response(ctx filters.FilterContext) {
	result, err := w.response.Call(context.Background(), 3, 2)
	if err != nil {
		log.Errorf("failed to call add: %v", err)
	}
	log.Infof("response result: %v", result)

}

func (w *wasm) Close() error {
	return w.runtime.Close(context.Background())
}
