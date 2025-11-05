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
	once     sync.Once
	mu       sync.Mutex
	registry *OpenPolicyAgentRegistry

	bundleMap map[string]struct{}

	log logging.Logger
}

// NewPreProcessor creates a pre-processor that pre-loads OPA instances
// Only used when pre-loading is enabled via command line flag
func (registry *OpenPolicyAgentRegistry) NewPreProcessor() routing.PreProcessor {
	return &opaPreProcessor{
		registry:  registry,
		bundleMap: make(map[string]struct{}),

		log: &logging.DefaultLog{},
	}
}

// Do implements routing.PreProcessor
func (p *opaPreProcessor) Do(routes []*eskip.Route) []*eskip.Route {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Extract OPA bundle requirements from routes
	bundleConfigs := p.extractOpaBundleRequests(routes)

	p.once.Do(func() {
		// On initial load, start all instances in parallel and wait for completion
		p.preloadInstancesParallel(bundleConfigs)
	})

	// check already processed bundles
	bundles := make([]string, 0, len(bundleConfigs))
	for _, bundleName := range bundleConfigs {
		if _, ok := p.bundleMap[bundleName]; !ok {
			bundles = append(bundles, bundleName)
			p.bundleMap[bundleName] = struct{}{}
		}
	}

	// For subsequent loads (or if no initial bundles), enqueue new instances for sequential processing
	if len(bundles) > 0 {
		p.enqueueInstancesSequential(bundles)
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
				p.log.Errorf("Failed to get existing OPA instance for bundle %q: %v", bundleName, err)
			}

			if inst != nil {
				if !inst.Started() {
					if err := inst.Start(); err != nil {
						p.log.Errorf("Failed to parallel start OPA instance for bundle %q: %v", bundleName, err)
					}
				}
				return
			}

			inst, err = p.registry.createAndCacheInstance(bundleName)
			if err != nil {
				p.log.Errorf("Failed to create OPA instance for bundle %q: %v", bundleName, err)
				return
			}
			err = inst.Start()
			if err != nil {
				p.log.Errorf("Failed to parallel start OPA instance for bundle %q: %v", bundleName, err)
				return
			}
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
			p.log.Errorf("Failed to get existing OPA instance for bundle %q: %v", bundle, err)
			continue
		}

		if inst != nil {
			if !inst.Started() {
				p.log.Info("Scheduling background task to start existing OPA instance for bundle: ", bundle)
				if _, err := p.registry.ScheduleBackgroundTask(inst.Start); err != nil {
					p.log.Errorf("Failed to reschedule OPA instance for bundle %q: %v", bundle, err)
				}
			}
			continue
		}

		inst, err = p.registry.createAndCacheInstance(bundle)
		if err != nil {
			p.log.Errorf("Failed to create OPA instance for bundle %q: %v", bundle, err)
			continue
		}

		// Schedule background task for sequential processing
		p.log.Info("Scheduling background task for new OPA instance for bundle: ", bundle)
		_, err = p.registry.ScheduleBackgroundTask(inst.Start)

		if err != nil {
			p.log.Errorf("Failed to schedule OPA instance for bundle %q: %v", bundle, err)
		}
	}
}
