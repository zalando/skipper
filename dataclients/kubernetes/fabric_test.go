package kubernetes

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCalculateTraffic(t *testing.T) {
	for _, tt := range []struct {
		name          string
		traffic       []*definitions.ActualTraffic
		wantTraffic   map[string]float64
		wantNoopCount int
	}{
		{
			name: "one entry",
			traffic: []*definitions.ActualTraffic{
				{
					StackName:   "app",
					ServiceName: "app",
					ServicePort: intstr.FromInt(8080),
					Weight:      1,
				},
			},
			wantTraffic: map[string]float64{
				"app": -1,
			},
			wantNoopCount: 0,
		},
		{
			name: "two entries 80/20",
			traffic: []*definitions.ActualTraffic{
				{
					StackName:   "app1",
					ServiceName: "app1",
					ServicePort: intstr.FromInt(8080),
					Weight:      80,
				},
				{
					StackName:   "app2",
					ServiceName: "app2",
					ServicePort: intstr.FromInt(8080),
					Weight:      20,
				},
			},
			wantTraffic: map[string]float64{
				"app1": 0.8,
				"app2": -1,
			},
			wantNoopCount: 0,
		},
		{
			name: "two entries one zero value",
			traffic: []*definitions.ActualTraffic{
				{
					StackName:   "app1",
					ServiceName: "app1",
					ServicePort: intstr.FromInt(8080),
					Weight:      80,
				},
				{
					StackName:   "app2",
					ServiceName: "app2",
					ServicePort: intstr.FromInt(8080),
					Weight:      0,
				},
			},
			wantTraffic: map[string]float64{
				"app1": -1,
			},
			wantNoopCount: 0,
		},
		{
			name: "three entries all same weight",
			traffic: []*definitions.ActualTraffic{
				{
					StackName:   "app1",
					ServiceName: "app1",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
				{
					StackName:   "app2",
					ServiceName: "app2",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
				{
					StackName:   "app3",
					ServiceName: "app3",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
			},
			wantTraffic: map[string]float64{
				"app1": 1.0 / 3.0,
				"app2": 0.5,
				"app3": -1,
			},
			wantNoopCount: 1,
		},
		{
			name: "three entries two same weight one zero value",
			traffic: []*definitions.ActualTraffic{
				{
					StackName:   "app1",
					ServiceName: "app1",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
				{
					StackName:   "app2",
					ServiceName: "app2",
					ServicePort: intstr.FromInt(8080),
					Weight:      0,
				},
				{
					StackName:   "app3",
					ServiceName: "app3",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
			},
			wantTraffic: map[string]float64{
				"app1": 0.5,
				"app3": -1,
			},
			wantNoopCount: 0,
		},
		{
			name: "three entries all different weights",
			traffic: []*definitions.ActualTraffic{
				{
					StackName:   "app1",
					ServiceName: "app1",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
				{
					StackName:   "app2",
					ServiceName: "app2",
					ServicePort: intstr.FromInt(8080),
					Weight:      30,
				},
				{
					StackName:   "app3",
					ServiceName: "app3",
					ServicePort: intstr.FromInt(8080),
					Weight:      20,
				},
			},
			wantTraffic: map[string]float64{
				"app1": 0.5,
				"app2": 0.6,
				"app3": -1,
			},
			wantNoopCount: 1,
		},
		{
			name: "four entries all same weight",
			traffic: []*definitions.ActualTraffic{
				{
					StackName:   "app1",
					ServiceName: "app1",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
				{
					StackName:   "app2",
					ServiceName: "app2",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
				{
					StackName:   "app3",
					ServiceName: "app3",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
				{
					StackName:   "app4",
					ServiceName: "app4",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
			},
			wantTraffic: map[string]float64{
				"app1": 0.25,
				"app2": 1.0 / 3.0,
				"app3": 0.5,
				"app4": -1,
			},
			wantNoopCount: 2,
		},
		{
			name: "four entries all different weights",
			traffic: []*definitions.ActualTraffic{
				{
					StackName:   "app1",
					ServiceName: "app1",
					ServicePort: intstr.FromInt(8080),
					Weight:      50,
				},
				{
					StackName:   "app2",
					ServiceName: "app2",
					ServicePort: intstr.FromInt(8080),
					Weight:      20,
				},
				{
					StackName:   "app3",
					ServiceName: "app3",
					ServicePort: intstr.FromInt(8080),
					Weight:      15,
				},
				{
					StackName:   "app4",
					ServiceName: "app4",
					ServicePort: intstr.FromInt(8080),
					Weight:      15,
				},
			},
			wantTraffic: map[string]float64{
				"app1": 0.5,
				"app2": 0.4,
				"app3": 0.5,
				"app4": -1,
			},
			wantNoopCount: 2,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			gotTraffic, gotNoopCount := calculateTrafficForStackset(tt.traffic)
			if gotNoopCount != tt.wantNoopCount {
				t.Errorf("Failed to calculate traffic: noopcount want %v, got %v", tt.wantNoopCount, gotNoopCount)
			}

			if len(tt.wantTraffic) != len(gotTraffic) {
				t.Errorf("Failed to get the same amount of traffic items (%d/%d): %v", len(tt.wantTraffic), len(gotTraffic), cmp.Diff(tt.wantTraffic, gotTraffic))
			}

			for k, v := range tt.wantTraffic {
				if v != gotTraffic[k] {
					t.Errorf("Failed to get same traffic for k %s: want: %v, got: %v", k, v, gotTraffic[k])
				}
			}
		})
	}

}
