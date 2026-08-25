package swarm

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestSwarm(t *testing.T) {
	o := Options{
		MaxMessageBuffer: DefaultMaxMessageBuffer,
		LeaveTimeout:     DefaultLeaveTimeout,
	}
	ep1 := &knownEntryPoint{
		self: &NodeInfo{Name: "first", Port: 9933},
	}
	ep2 := &knownEntryPoint{
		self: &NodeInfo{Name: "second", Port: 9934},
	}
	ep3 := &knownEntryPoint{
		self: &NodeInfo{Name: "third", Port: 9935},
	}
	all := []*NodeInfo{ep1.Node(), ep2.Node(), ep3.Node()}
	cleanupF := func() {}

	first, err := Join(o, ep1.Node(), all, cleanupF)
	if err != nil {
		t.Fatalf("Failed to start first: %v", err)
	}
	second, err := Join(o, ep2.Node(), all, cleanupF)
	if err != nil {
		t.Fatalf("Failed to start second: %v", err)
	}
	third, err := Join(o, ep3.Node(), all, cleanupF)
	if err != nil {
		t.Fatalf("Failed to start third: %v", err)
	}

	first.ShareValue("foo", 1)
	second.ShareValue("foo", 2)
	second.ShareValue("bar", 3)
	third.ShareValue("bar", 4)

	const delay = 300 * time.Millisecond
	time.Sleep(delay)

	checkValues := func(s []*Swarm, key string, expected map[string]any) {
		for _, si := range s {
			got := si.Values(key)
			if !cmp.Equal(got, expected) {
				t.Errorf("invalid state: %v", cmp.Diff(got, expected))
				return
			}
		}
	}

	checkValues([]*Swarm{first, second, third}, "foo", map[string]any{
		first.Local().Name:  1,
		second.Local().Name: 2,
	})

	checkValues([]*Swarm{first, second, third}, "bar", map[string]any{
		second.Local().Name: 3,
		third.Local().Name:  4,
	})
}
