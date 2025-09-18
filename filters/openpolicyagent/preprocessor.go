package openpolicyagent

import (
	"slices"
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

	log logging.Logger
}

// NewPreProcessor creates a pre-processor that pre-loads OPA instances
// Only used when pre-loading is enabled via command line flag
func (registry *OpenPolicyAgentRegistry) NewPreProcessor() routing.PreProcessor {
	return &opaPreProcessor{
		registry: registry,

		log: &logging.DefaultLog{},
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
func (p *opaPreProcessor) extractOpaBundleRequests(routes []*eskip.Route) []string {
	requirements := []string{}

	for _, route := range routes {
		for _, filter := range route.Filters {
			if p.isOpaFilter(filter.Name) && len(filter.Args) > 0 {
				if bundleName, ok := filter.Args[0].(string); ok {
					if !slices.Contains(requirements, bundleName) {
						requirements = append(requirements, bundleName)
					}
				}
			}
		}
	}

	return requirements
}

func (p *opaPreProcessor) isOpaFilter(filterName string) bool {
	return strings.HasPrefix(filterName, "opa")
}

func (p *opaPreProcessor) preloadInstancesParallel(bundles []string) {
	var wg sync.WaitGroup

	for _, req := range bundles {
		wg.Add(1)
		go func(bundleName string) {
			defer wg.Done()

			inst, err := p.registry.getExistingInstance(bundleName)
			if err != nil {
				p.log.Errorf("Failed to get existing OPA instance for bundle '%s': %v", bundleName, err)
			}

			if inst != nil {
				// Instance already ready, skip
				return
			}

			inst, err = p.registry.createAndCacheInstance(bundleName)
			if err != nil {
				p.log.Errorf("Failed to create OPA instance for bundle '%s': %v", bundleName, err)
				return
			}
			err = inst.Start()
			if err != nil {
				p.log.Errorf("Failed to start OPA instance for bundle '%s': %v", bundleName, err)
				return
			}

			p.log.Infof("Successfully preloaded OPA instance for bundle '%s'", bundleName)
		}(req)
	}

	// Wait for all instances to complete
	wg.Wait()
}

// enqueueInstancesSequential enqueues new instances for sequential processing using background tasks
func (p *opaPreProcessor) enqueueInstancesSequential(bundles []string) {
	for _, bundle := range bundles {
		// Check if instance already exists to avoid unnecessary work
		inst, err := p.registry.getExistingInstance(bundle)

		if err != nil {
			p.log.Errorf("Failed to get existing OPA instance for bundle '%s': %v", bundle, err)
			continue
		}

		if inst != nil {
			if !inst.started {
				if err := inst.Start(); err != nil {
					p.log.Errorf("Failed to start OPA instance for bundle '%s': %v", bundle, err)
				}
			}
			continue
		}

		inst, err = p.registry.createAndCacheInstance(bundle)
		if err != nil {
			p.log.Errorf("Failed to create OPA instance for bundle '%s': %v", bundle, err)
			continue
		}

		// Schedule background task for sequential processing
		_, err = p.registry.ScheduleBackgroundTask(inst.Start)

		if err != nil {
			p.log.Errorf("Failed to schedule OPA instance for bundle '%s': %v", bundle, err)
		}

		p.log.Infof("Scheduled OPA instance for bundle '%s' for background loading", bundle)
	}
}
