package scheduler_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
)

func TestScheduler(t *testing.T) {
	fr := builtin.MakeRegistry()

	for _, tt := range []struct {
		name    string
		doc     string
		paths   [][]string
		wantErr bool
	}{
		{
			name:    "no filter",
			doc:     `r0: * -> "http://www.example.org"`,
			wantErr: true,
		},
		{
			name:    "one filter without scheduler filter",
			doc:     `r1: * -> setPath("/bar") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "one scheduler filter lifo",
			doc:     `l2: * -> lifo(10, 12, "10s") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "one scheduler filter lifoGroup",
			doc:     `r2: * -> lifoGroup("r2", 10, 12, "10s") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "multiple filters with one scheduler filter lifo",
			doc:     `l3: * -> setPath("/bar") -> lifo(10, 12, "10s") -> setRequestHeader("X-Foo", "bar") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "multiple filters with one scheduler filter lifoGroup",
			doc:     `r3: * -> setPath("/bar") -> lifoGroup("r3", 10, 12, "10s") -> setRequestHeader("X-Foo", "bar") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "multiple routes with lifo filters do not interfere",
			doc:     `l4: Path("/l4") -> setPath("/bar") -> lifo(10, 12, "10s") -> "http://www.example.org"; l5: Path("/l5") -> setPath("/foo") -> lifo(15, 2, "11s")  -> setRequestHeader("X-Foo", "bar")-> "http://www.example.org";`,
			paths:   [][]string{[]string{"l4"}, []string{"l5"}},
			wantErr: false,
		},
		{
			name:    "multiple routes with different grouping do not interfere",
			doc:     `r4: Path("/r4") -> setPath("/bar") -> lifoGroup("r4", 10, 12, "10s") -> "http://www.example.org"; r5: Path("/r5") -> setPath("/foo") -> lifoGroup("r5", 15, 2, "11s")  -> setRequestHeader("X-Foo", "bar")-> "http://www.example.org";`,
			paths:   [][]string{[]string{"r4"}, []string{"r5"}},
			wantErr: false,
		},
		{
			name:    "multiple routes with same grouping do use the same configuration",
			doc:     `r6: Path("/r6") -> setPath("/bar") -> lifoGroup("r6", 10, 12, "10s") -> "http://www.example.org"; r7: Path("/r7") -> setPath("/foo") -> lifoGroup("r6", 10, 12, "10s")  -> setRequestHeader("X-Foo", "bar")-> "http://www.example.org";`,
			wantErr: false,
			paths:   [][]string{{"r6", "r7"}},
		}} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := testdataclient.NewDoc(tt.doc)
			if err != nil {
				t.Fatalf("Failed to create a test dataclient: %v", err)
			}

			reg := scheduler.NewRegistry()
			ro := routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  fr,
				DataClients:     []routing.DataClient{cli},
				PostProcessors: []routing.PostProcessor{
					reg,
				},
			}
			rt := routing.New(ro)
			defer rt.Close()
			<-rt.FirstLoad() // sync

			if len(tt.paths) == 0 {
				r, _ := rt.Route(&http.Request{URL: &url.URL{Path: "/foo"}})
				if r == nil {
					t.Errorf("Route is nil but we do not expect an error")
					return
				}

				for _, f := range r.Filters {
					if f == nil && !tt.wantErr {
						t.Errorf("Filter is nil but we do not expect an error")
					}
					lf, ok := f.Filter.(scheduler.LIFOFilter)
					if !ok {
						continue
					}
					cfg := lf.Config(reg)
					stack := lf.GetStack()
					if stack == nil {
						t.Errorf("Stack is nil")
					}
					if cfg != stack.Config() {
						t.Errorf("Failed to get stack with configuration, want: %v, got: %v", cfg, stack)
					}
				}
			}

			// tt.paths
			stacksMap := make(map[string][]*scheduler.Stack)

			for _, group := range tt.paths {
				key := group[0]

				for _, p := range group {
					r, _ := rt.Route(&http.Request{URL: &url.URL{Path: "/" + p}})
					if r == nil {
						t.Errorf("Route is nil but we do not expect an error, path: %s", p)
						return
					}

					for _, f := range r.Filters {
						if f == nil && !tt.wantErr {
							t.Errorf("Filter is nil but we do not expect an error")
						}
						lf, ok := f.Filter.(scheduler.LIFOFilter)
						if !ok {
							continue
						}
						cfg := lf.Config(reg)
						stack := lf.GetStack()
						if stack == nil {
							t.Errorf("Stack is nil")
						}
						if cfg != stack.Config() {
							t.Errorf("Failed to get stack with configuration, want: %v, got: %v", cfg, stack)
						}

						if lf.Key() != key {
							t.Errorf("Failed to get the right key: %s, expected: %s", lf.Key(), key)
						}
						k := lf.Key()
						stacksMap[k] = append(stacksMap[k], stack)
					}
				}
				if len(stacksMap[key]) != len(group) {
					t.Errorf("Failed to get the right group size %v != %v", len(stacksMap[key]), len(group))
				}
			}
			// check pointers to stack are the same for same group
			for k, stacks := range stacksMap {
				firstStack := stacks[0]
				for _, stack := range stacks {
					if stack != firstStack {
						t.Errorf("Unexpected different stack in group: %s", k)
					}
				}
			}
			// check pointers to stack of different groups are different
			diffStacks := make(map[*scheduler.Stack]struct{})
			for _, stacks := range stacksMap {
				diffStacks[stacks[0]] = struct{}{}
			}
			if len(diffStacks) != len(stacksMap) {
				t.Error("Unexpected got pointer to the same stack for different group")
			}
		})
	}

}
