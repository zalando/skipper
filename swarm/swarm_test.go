package swarm

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestInitializeSwarm(t *testing.T) {
	ep1 := &KnownEPoint{
		self: &NodeInfo{Name: "first", Port: 9933},
	}
	ep2 := &KnownEPoint{
		self: &NodeInfo{Name: "second", Port: 9934},
	}
	ep3 := &KnownEPoint{
		self: &NodeInfo{Name: "third", Port: 9935},
	}
	all := []*NodeInfo{ep1.Node(), ep2.Node(), ep3.Node()}

	first, err := Join(Options{}, ep1.Node(), all)
	if err != nil {
		t.Fatalf("Failed to start first: %v", err)
	}
	second, err := Join(Options{}, ep2.Node(), all)
	if err != nil {
		t.Fatalf("Failed to start second: %v", err)
	}
	third, err := Join(Options{}, ep3.Node(), all)
	if err != nil {
		t.Fatalf("Failed to start third: %v", err)
	}

	first.ShareValue("foo", 1)
	second.ShareValue("foo", 2)
	second.ShareValue("bar", 3)
	third.ShareValue("bar", 4)

	const delay = 300 * time.Millisecond
	time.Sleep(delay)

	checkValues := func(s []*Swarm, key string, expected map[string]interface{}) {
		for _, si := range s {
			got := si.Values(key)
			if !cmp.Equal(got, expected) {
				t.Errorf("invalid state: %v", cmp.Diff(got, expected))
				return
			}
		}
	}

	checkValues([]*Swarm{first, second, third}, "foo", map[string]interface{}{
		first.Local().Name:  1,
		second.Local().Name: 2,
	})

	checkValues([]*Swarm{first, second, third}, "bar", map[string]interface{}{
		second.Local().Name: 3,
		third.Local().Name:  4,
	})
}
