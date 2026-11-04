package auth

import (
	"encoding/json"
	"net/url"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/net"
)

// OpenIDConfigCache shares successful discovery documents by exact issuer URL.
// Instances must be constructed with NewOpenIDConfigCache.
type OpenIDConfigCache struct {
	mu      sync.RWMutex
	configs map[string]*openIDConfig

	closeOnce sync.Once
	quit      chan struct{}
	done      chan struct{}
}

// NewOpenIDConfigCache creates an isolated discovery cache and starts its hourly
// whole-map cleaner. The owner must Close it after its users and routes tear down.
func NewOpenIDConfigCache() *OpenIDConfigCache {
	c := &OpenIDConfigCache{
		configs: make(map[string]*openIDConfig),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go c.clean()
	return c
}

func getOpenIDConfig(issuerURL string, options TokenintrospectionOptions) (*openIDConfig, error) {
	if options.OpenIDConfigCache == nil {
		return fetchOpenIDConfig(issuerURL, options)
	}
	return options.OpenIDConfigCache.get(issuerURL, options)
}

// Cached discovery documents are shared and must remain read-only.
func (c *OpenIDConfigCache) get(issuerURL string, options TokenintrospectionOptions) (*openIDConfig, error) {
	c.mu.RLock()
	cfg, ok := c.configs[issuerURL]
	c.mu.RUnlock()
	if ok {
		return cfg, nil
	}
	return c.load(issuerURL, options)
}

func (c *OpenIDConfigCache) load(issuerURL string, options TokenintrospectionOptions) (*openIDConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cfg, ok := c.configs[issuerURL]; ok {
		return cfg, nil
	}
	cfg, err := fetchOpenIDConfig(issuerURL, options)
	if err != nil {
		return nil, err
	}
	c.configs[issuerURL] = cfg
	return cfg, nil
}

func fetchOpenIDConfig(issuerURL string, options TokenintrospectionOptions) (*openIDConfig, error) {
	u, err := url.Parse(issuerURL + TokenIntrospectionConfigPath)
	if err != nil {
		return nil, err
	}

	cli := newOpenIDConfigClient(options)
	defer cli.Close()

	rsp, err := cli.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != 200 {
		return nil, errInvalidToken
	}
	var decoded openIDConfig
	if err := json.NewDecoder(rsp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return &decoded, nil
}

func (c *OpenIDConfigCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs = make(map[string]*openIDConfig)
}

// Close stops the hourly cleaner and waits for it to exit. It is idempotent and
// safe for concurrent callers. The owner must tear down all users and routes
// before closing the cache.
func (c *OpenIDConfigCache) Close() {
	c.closeOnce.Do(func() { close(c.quit) })
	<-c.done
}

func (c *OpenIDConfigCache) clean() {
	ticker := time.NewTicker(time.Hour)
	defer close(c.done)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.clear()
		case <-c.quit:
			return
		}
	}
}

func newOpenIDConfigClient(o TokenintrospectionOptions) *net.Client {
	if o.Tracer == nil {
		o.Tracer = opentracing.NoopTracer{}
	}
	if o.MaxIdleConns <= 0 {
		o.MaxIdleConns = defaultMaxIdleConns
	}

	timeout := o.openIDConfigTimeout()
	return net.NewClient(net.Options{
		Timeout:                 timeout,
		ResponseHeaderTimeout:   timeout,
		TLSHandshakeTimeout:     timeout,
		MaxIdleConnsPerHost:     o.MaxIdleConns,
		Tracer:                  o.Tracer,
		OpentracingComponentTag: "skipper",
		OpentracingSpanName:     openIDConfigSpanName,
		OpentracingEventsByTag:  o.OpenTracingClientTraceByTag,
	})
}
