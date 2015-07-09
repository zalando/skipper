package filters

import "github.com/zalando/skipper/skipper"

// default implementation of skipper.FilterRegistry
type registry struct {
	mw map[string]skipper.FilterSpec
}

func makeRegistry() skipper.FilterRegistry {
	return &registry{map[string]skipper.FilterSpec{}}
}

func (r *registry) Add(mw ...skipper.FilterSpec) {
	for _, mwi := range mw {
		r.mw[mwi.Name()] = mwi
	}
}

func (r *registry) Get(name string) skipper.FilterSpec {
	return r.mw[name]
}

func (r *registry) Remove(name string) {
	delete(r.mw, name)
}
