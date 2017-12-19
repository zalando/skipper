package swarm

import (
	"reflect"
	"testing"
	"time"

	"github.com/sanity-io/litter"
)

func TestInitializeSwarm(t *testing.T) {
	first, err := Start(
		Options{NodeSpec: &NodeInfo{Name: "first", Port: 9933}},
	)
	if err != nil {
		t.Fatal(err)
	}

	entryPoint := KnownEntryPoint(first.Local())

	second, err := Join(
		Options{NodeSpec: &NodeInfo{Name: "second", Port: 9934}},
		entryPoint,
	)
	if err != nil {
		t.Fatal(err)
	}

	third, err := Join(
		Options{NodeSpec: &NodeInfo{Name: "third", Port: 9935}},
		entryPoint,
	)
	if err != nil {
		t.Fatal(err)
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
			if !reflect.DeepEqual(got, expected) {
				t.Error("invalid state")
				t.Log("got:     ", litter.Sdump(got))
				t.Log("expected:", litter.Sdump(expected))
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
