package definitions

import (
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/predicates/host"
	"github.com/zalando/skipper/predicates/methods"
	"github.com/zalando/skipper/routing"
)

func TestValidateRoutegroup(t *testing.T) {

	for _, tt := range []struct {
		name                     string
		routingOptions           routing.Options
		enableAdvancedValidation bool
		rg                       *RouteGroupItem
		wantErr                  bool
	}{
		{
			name:                     "test no routingOptions",
			enableAdvancedValidation: false,
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "test empty routingOptions",
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  false,
		},
		{
			name: "test empty routingOptions advanced",
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  false,
		},
		{
			name: "test unknown filter",
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
							Filters: []string{
								`setRequestHeader("Foo", "foo")`,
								`unknownFilter("Foo", "foo")`,
							},
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  false,
		},
		{
			name: "test unknown filter advanced",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
			},
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
							Filters: []string{
								`setRequestHeader("Foo", "foo")`,
								`unknownFilter("Foo", "foo")`,
							},
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "test 2 unknown filters advanced",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
			},
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
							Filters: []string{
								`setRequestHeader("Foo", "foo")`,
								`unknownFilter1("Foo", "foo")`,
								`unknownFilter2("Foo", "foo")`,
							},
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "test bad filter params",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
			},
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
							Filters: []string{
								`setRequestHeader(3, "Foo", "foo")`,
							},
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "test bad filter parse error",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
			},
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
							Filters: []string{
								`setRequestHeader("Foo", "foo"`,
							},
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "test bad predicate unknown and parse error",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
			},
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name: "shunt",
							Type: eskip.ShuntBackend,
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "shunt",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
							Predicates: []string{
								`Host("foo.example"`,
								`UknownPredicate("foo.example")`,
							},
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "test bad backend url parse error",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
			},
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name:    "be",
							Type:    eskip.NetworkBackend,
							Address: "http:/test.example",
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "be",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "test backend url ok",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
			},
			rg: &RouteGroupItem{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "rg1",
				},
				Spec: &RouteGroupSpec{
					Hosts: []string{"rgv1.example"},
					Backends: []*SkipperBackend{
						{
							Name:    "be",
							Type:    eskip.NetworkBackend,
							Address: "http://test.example",
						},
					},
					DefaultBackends: BackendReferences{
						&BackendReference{
							BackendName: "be",
						},
					},
					Routes: []*RouteSpec{
						{
							PathSubtree: "/",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			rgv := RouteGroupValidator{
				RoutingOptions:           tt.routingOptions,
				EnableAdvancedValidation: tt.enableAdvancedValidation,
			}

			if err := rgv.Validate(tt.rg); err != nil && !tt.wantErr {
				t.Fatalf("Failed to validate: %v", err)
			}
		})
	}
}

func TestValidateIngress(t *testing.T) {

	for _, tt := range []struct {
		name                     string
		routingOptions           routing.Options
		enableAdvancedValidation bool
		ing                      *IngressV1Item
		wantErr                  bool
	}{
		{
			name: "no routingOptions, advanced false, no annotations",
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace:   "ns1",
					Name:        "ing1",
					Annotations: map[string]string{},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  false,
		},
		{
			name: "no routingOptions, advanced true, no annotations",
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace:   "ns1",
					Name:        "ing1",
					Annotations: map[string]string{},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  false,
		},
		{
			name: "advanced false, all annotations ok",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  false,
		},
		{
			name: "advanced true, all annotations ok",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  false,
		},
		{
			name: "advanced false, filter annotations wrong but eskip ok",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setFoo("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  false,
		},
		{
			name: "advanced false, filter annotation eskip wrong",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader(X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  true,
		},
		{
			name: "advanced true, filter annotation wrong",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setFoo("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "advanced false, predicate annotations wrong but eskip ok",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `Foo("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  false,
		},
		{
			name: "advanced false, predicate annotation eskip wrong",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny(www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  true,
		},
		{
			name: "advanced true, predicate annotation wrong",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `Host("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "advanced true, predicate annotation wrong argument",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny(5, "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "advanced false, routes annotations wrong but eskip ok",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Meth("OPTIONS") -> stat(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  false,
		},
		{
			name: "advanced false, routes annotation eskip wrong",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <shunt`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: false,
			wantErr:                  true,
		},
		{
			name: "advanced true, routes annotation wrong backend",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `Host("www.example.org", "www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status(200) -> <foo>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "advanced true, routes annotation wrong argument",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> status("200") -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "advanced true, routes annotation wrong filter",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.com")`,
						IngressRoutesAnnotation:    `Methods("OPTIONS") -> foo("") -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		},
		{
			name: "advanced true, routes annotation wrong predicate",
			routingOptions: routing.Options{
				Metrics:        metrics.Default,
				FilterRegistry: builtin.MakeRegistry(),
				Predicates: []routing.PredicateSpec{
					host.NewAny(),
					methods.New(),
				},
			},
			ing: &IngressV1Item{
				Metadata: &Metadata{
					Namespace: "ns1",
					Name:      "ing1",
					Annotations: map[string]string{
						IngressFilterAnnotation:    `setRequestHeader("X-Passed-Skipper", "true")`,
						IngressPredicateAnnotation: `HostAny("www.example.com")`,
						IngressRoutesAnnotation:    `Meth("OPTIONS") -> status(200) -> <shunt>`,
					},
				},
				Spec: &IngressV1Spec{
					Rules: []*RuleV1{
						{
							Host: "ing.example",
						},
					},
				},
			},
			enableAdvancedValidation: true,
			wantErr:                  true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			ing := IngressV1Validator{
				RoutingOptions:           tt.routingOptions,
				EnableAdvancedValidation: tt.enableAdvancedValidation,
			}

			if err := ing.Validate(tt.ing); err != nil && !tt.wantErr {
				t.Fatalf("Failed to validate: %v", err)
			} else if tt.wantErr && err == nil {
				t.Fatalf("Failed to find error on validate: %v", err)
			}
		})
	}
}
