package metrics

import (
	"reflect"
	"testing"
)

type cacheL2TestProvider struct {
	Metrics
	operations [][3]string
	errors     [][3]string
}

func (p *cacheL2TestProvider) IncCacheL2Operation(operation, purpose, outcome string) {
	p.operations = append(p.operations, [3]string{operation, purpose, outcome})
}

func (p *cacheL2TestProvider) IncCacheL2ProcessingError(operation, purpose, stage string) {
	p.errors = append(p.errors, [3]string{operation, purpose, stage})
}

func TestAllCacheL2Observer(t *testing.T) {
	first := &cacheL2TestProvider{Metrics: NewVoid()}
	second := &cacheL2TestProvider{Metrics: NewVoid()}
	// A separately stored Prometheus provider must not receive an extra call.
	prom := NewPrometheus(Options{})
	nested := &All{providers: []Metrics{NewVoid(), second}}
	all := &All{providers: []Metrics{first, NewVoid(), nested, prom}, prometheus: prom}

	all.IncCacheL2Operation("get", "response", "success")
	all.IncCacheL2ProcessingError("set", "metadata", "encode")
	for _, provider := range []*cacheL2TestProvider{first, second} {
		if want := [][3]string{{"get", "response", "success"}}; !reflect.DeepEqual(provider.operations, want) {
			t.Errorf("operations = %v, want %v", provider.operations, want)
		}
		if want := [][3]string{{"set", "metadata", "encode"}}; !reflect.DeepEqual(provider.errors, want) {
			t.Errorf("errors = %v, want %v", provider.errors, want)
		}
	}
	families, err := prom.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, family := range families {
		switch family.GetName() {
		case "skipper_cache_l2_operations_total", "skipper_cache_l2_processing_errors_total":
			found++
			if len(family.Metric) != 1 || family.Metric[0].GetCounter().GetValue() != 1 {
				t.Errorf("%s: got %v, want one increment", family.GetName(), family.Metric)
			}
		}
	}
	if found != 2 {
		t.Errorf("found %d L2 metric families, want 2", found)
	}
}
