package auth

import (
	"encoding/json"
	"net/url"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/net"
)

type openIDConfigCache struct {
	mu      sync.RWMutex
	configs map[string]*openIDConfig

	cleanerMu sync.Mutex
	owners    int
	quit      chan struct{}
	done      chan struct{}
}

var openIDConfigs = &openIDConfigCache{configs: make(map[string]*openIDConfig)}

func getOpenIDConfig(issuerURL string, options TokenintrospectionOptions) (*openIDConfig, error) {
	if !options.EnableOpenIDConfigCache {
		return fetchOpenIDConfig(issuerURL, options)
	}
	return openIDConfigs.get(issuerURL, options)
}

// Cached discovery documents are shared and must remain read-only.
func (c *openIDConfigCache) get(issuerURL string, options TokenintrospectionOptions) (*openIDConfig, error) {
	c.mu.RLock()
	cfg, ok := c.configs[issuerURL]
	c.mu.RUnlock()
	if ok {
		return cfg, nil
	}
	return c.load(issuerURL, options)
}

func (c *openIDConfigCache) load(issuerURL string, options TokenintrospectionOptions) (*openIDConfig, error) {
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

func (c *openIDConfigCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs = make(map[string]*openIDConfig)
}

// StartOpenIDConfigCacheCleaner starts hourly whole-map discovery cleanup.
// All owners share one cleaner. The returned idempotent function releases the
// owner; the last release stops the ticker and waits for the cleaner to exit.
// Library owners enabling TokenintrospectionOptions.EnableOpenIDConfigCache
// must acquire a cleaner and release it after routing teardown. Constructors
// do not start it; without an owner, enabled discovery caching has no automatic
// clearing. Disabled discovery bypasses the cache and does not need a cleaner.
func StartOpenIDConfigCacheCleaner() func() {
	return openIDConfigs.startCleaner()
}

func (c *openIDConfigCache) startCleaner() func() {
	c.cleanerMu.Lock()
	defer c.cleanerMu.Unlock()
	if c.owners == 0 {
		c.quit = make(chan struct{})
		c.done = make(chan struct{})
		go c.clean()
	}
	c.owners++

	var once sync.Once
	return func() {
		once.Do(func() {
			c.cleanerMu.Lock()
			defer c.cleanerMu.Unlock()
			c.owners--
			if c.owners == 0 {
				close(c.quit)
				<-c.done
			}
		})
	}
}

func (c *openIDConfigCache) clean() {
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
