package kubernetes

import (
	"reflect"
	"testing"
)

func Test_endpoint_Targets(t *testing.T) {
	tests := []struct {
		name       string
		Subsets    []*subset
		ingSvcPort string
		targetPort *backendPort
		want       []string
	}{
		{
			name: "single node and port fully specified by name",
			Subsets: []*subset{
				&subset{
					Addresses: []*address{
						&address{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						&port{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "http",
			targetPort: &backendPort{value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and port fully specified by port number",
			Subsets: []*subset{
				&subset{
					Addresses: []*address{
						&address{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						&port{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "80",
			targetPort: &backendPort{value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and 2 ports fully specified by name",
			Subsets: []*subset{
				&subset{
					Addresses: []*address{
						&address{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						&port{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
						&port{
							Name:     "metrics",
							Port:     9911,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "http",
			targetPort: &backendPort{value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and 2 ports fully specified by port number",
			Subsets: []*subset{
				&subset{
					Addresses: []*address{
						&address{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						&port{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
						&port{
							Name:     "metrics",
							Port:     9911,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "80",
			targetPort: &backendPort{value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := endpoint{
				Subsets: tt.Subsets,
			}
			if got := ep.Targets(tt.ingSvcPort, tt.targetPort.String()); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("endpoint.Targets() = %v, want %v", got, tt.want)
			}
		})
	}
}
