package kubernetes

import (
	"reflect"
	"testing"

	"github.com/zalando/skipper/eskip"
)

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
