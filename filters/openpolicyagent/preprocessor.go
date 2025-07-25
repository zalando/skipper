package openpolicyagent

import (
	"strings"
	"sync"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/routing"
)

type opaPreProcessor struct {
	registry    *OpenPolicyAgentRegistry
	initialLoad sync.Once
	mu          sync.Mutex
}

// NewPreProcessor creates a pre-processor that pre-loads OPA instances
// Only used when pre-loading is enabled via command line flag
func (registry *OpenPolicyAgentRegistry) NewPreProcessor() routing.PreProcessor {
	return &opaPreProcessor{
		registry: registry,
	}
}

// Do implements routing.PreProcessor
func (p *opaPreProcessor) Do(routes []*eskip.Route) []*eskip.Route {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Extract OPA bundle requirements from routes
	bundleConfigs := p.extractOpaBundleRequests(routes)

	// Use sync.Once to ensure initial load happens exactly once
	p.initialLoad.Do(func() {
		// On initial load, start all instances in parallel and wait for completion
		p.preloadInstancesParallel(bundleConfigs)
	})

	// For subsequent loads (or if no initial bundles), enqueue new instances for sequential processing
	if len(bundleConfigs) > 0 {
		p.enqueueInstancesSequential(bundleConfigs)
	}

	return routes
}

// extractOpaBundleRequests scans routes for OPA filter usage and extracts bundle requirements
func (p *opaPreProcessor) extractOpaBundleRequests(routes []*eskip.Route) map[string]bundleRequest {
	requirements := make(map[string]bundleRequest)

	for _, route := range routes {
		for _, filter := range route.Filters {
			if p.isOpaFilter(filter.Name) && len(filter.Args) > 0 {
				if bundleName, ok := filter.Args[0].(string); ok {
					req := bundleRequest{
						bundleName: bundleName,
						filterName: filter.Name,
					}
					requirements[bundleName] = req
				}
			}
		}
	}

	return requirements
}

type bundleRequest struct {
	bundleName string
	filterName string
}

func (p *opaPreProcessor) isOpaFilter(filterName string) bool {
	return strings.HasPrefix(filterName, "opa")
}

func (p *opaPreProcessor) preloadInstancesParallel(requests map[string]bundleRequest) {
	var wg sync.WaitGroup
	log := &logging.DefaultLog{}

	for _, req := range requests {
		wg.Add(1)
		go func(r bundleRequest) {
			defer wg.Done()

			// Use the new PrepareInstanceLoader approach
			loader := p.registry.PrepareInstanceLoader(r.bundleName, r.filterName)
			_, err := loader()
			if err != nil {
				log.Errorf("Failed to load OPA instance for bundle '%s': %v", r.bundleName, err)
			} else {
				log.Debugf("Successfully preloaded OPA instance for bundle '%s'", r.bundleName)
			}
		}(req)
	}

	// Wait for all instances to complete
	wg.Wait()
}

// enqueueInstancesSequential enqueues new instances for sequential processing using background tasks
func (p *opaPreProcessor) enqueueInstancesSequential(requests map[string]bundleRequest) {
	log := &logging.DefaultLog{}

	for _, req := range requests {
		// Check if instance already exists to avoid unnecessary work
		_, err := p.registry.GetOrStartInstance(req.bundleName, req.filterName)
		if err == nil {
			// Instance already ready, skip
			continue
		}

		// Schedule background task for sequential processing
		_, err = p.registry.ScheduleBackgroundTask(func() (interface{}, error) {
			return p.registry.PrepareInstanceLoader(req.bundleName, req.filterName)()
		})

		if err != nil {
			log.Errorf("Failed to schedule OPA instance for bundle '%s': %v", req.bundleName, err)
			continue
		}

		log.Debugf("Scheduled OPA instance for bundle '%s' for background loading", req.bundleName)
	}
}
