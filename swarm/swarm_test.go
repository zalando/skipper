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

	const delay = 999 * time.Millisecond
	time.Sleep(6 * delay)

	first.ShareValue("foo", 1)
	time.Sleep(delay)

	second.ShareValue("foo", 2)
	time.Sleep(delay)

	second.ShareValue("bar", 3)
	time.Sleep(delay)

	third.ShareValue("bar", 4)
	time.Sleep(delay)

	litter.Dump(first.shared)
	litter.Dump(second.shared)
	litter.Dump(third.shared)

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
