package kubernetes

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	skippernet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/predicates/host"
	"github.com/zalando/skipper/routing"
)

func TestIngressDefaultBackendEastWestHost(t *testing.T) {
	ingresses := &definitions.IngressV1List{Items: []*definitions.IngressV1Item{
		testIngress("namespace1", "ingress1", "service1", "", "", "", "", "", "", nil, definitions.BackendPortV1{Number: 8080}),
		testIngress("namespace1", "ingress2", "service2", "", "", "", "", "", "", nil, definitions.BackendPortV1{Number: 8181}),
	}}
	api := newTestAPI(t, testServices(), ingresses)
	defer api.Close()
	client, err := New(Options{
		KubernetesURL: api.server.URL, KubernetesEnableEastWest: true,
		KubernetesEastWestDomain: "skipper.cluster.local", ForceKubernetesService: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	routes, err := client.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := routing.NewRoutesMatcher(&routing.Options{
		FilterRegistry: builtin.MakeRegistry(), Predicates: []routing.PredicateSpec{host.NewAny()},
	}, routes)
	if err != nil {
		t.Fatal(err)
	}
	patch := skippernet.HostPatch{RemovePort: true, RemoteTrailingDot: true, ToLower: true}
	for _, suffix := range []string{"", ".", ":80", ".:443"} {
		req := httptest.NewRequest("GET", "http://example.org/", nil)
		req.Host = patch.Apply("namespace1.ingress2.skipper.cluster.local" + suffix)
		route, _ := matcher.Match(req)
		if route == nil || route.Id != "kubeew_namespace1__ingress2______" || route.Backend != "http://service2.namespace1.svc.cluster.local:8181" {
			t.Errorf("host %q: got %v, want the East-West route to service2", req.Host, route)
		}
	}
}

func TestCreateEastWestRouteIng(t *testing.T) {
	type args struct {
		eastWestDomain string
		hostname       string
		namespace      string
		route          *eskip.Route
	}
	tests := []struct {
		name string
		args args
		want *eskip.Route
	}{
		{
			name: "return nil if route.Id is prefixed with 'kubeew'",
			args: args{
				eastWestDomain: "yyy",
				hostname:       "serviceA",
				namespace:      "A",
				route: &eskip.Route{
					Id: "kubeew_foo__qux_a_0__www2_example_org_____",
				},
			},
			want: nil,
		},
		{
			name: "return nil if the namespace is empty",
			args: args{
				eastWestDomain: "yyy",
				hostname:       "serviceA",
				namespace:      "",
				route: &eskip.Route{
					Id: "kube_foo__qux__www3_example_org___a_path__bar",
				},
			},
			want: nil,
		},
		{
			name: "return nil if the hostname is empty",
			args: args{
				eastWestDomain: "yyy",
				hostname:       "",
				namespace:      "A",
				route: &eskip.Route{
					Id: "kube_foo__qux__www3_example_org___a_path__bar",
				},
			},
			want: nil,
		},
		{
			name: "return the route with modified route.Id and HostRegexp",
			args: args{
				eastWestDomain: "cluster.local",
				hostname:       "serviceA",
				namespace:      "default",
				route: &eskip.Route{
					Id:          "kube_foo__qux__www3_example_org___a_path__bar",
					HostRegexps: []string{"www2[.]example[.]org"},
				},
			},
			want: &eskip.Route{
				Id:          "kubeew_foo__qux__www3_example_org___a_path__bar",
				HostRegexps: []string{"^(serviceA[.]default[.]cluster[.]local[.]?(:[0-9]+)?)$"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eastWestRouteIng := createEastWestRouteIng(tt.args.eastWestDomain, tt.args.hostname, tt.args.namespace, tt.args.route)
			if !reflect.DeepEqual(eastWestRouteIng, tt.want) {
				t.Errorf("createEastWestRouteIng() = %v, want %v", eastWestRouteIng, tt.want)
			}
		})
	}
}
