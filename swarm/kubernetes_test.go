package swarm

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/sanity-io/litter"
)

func TestKubernetesSwarm(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(content))
	}))

	entry := KubernetesEntry(KubernetesOptions{
		Client: NewNodeInfoClient(s.URL),
		hackPort: 8800,
	})
	first, err := Start(
		Options{SelfSpec: entry},
	)
	if err != nil {
		t.Fatal(err)
	}

	entry = KubernetesEntry(KubernetesOptions{
		Client: NewNodeInfoClient(s.URL),
		hackPort: 8801,
	})
	second, err := Join(
		Options{SelfSpec: entry},
		entry,
	)
	if err != nil {
		t.Fatal(err)
	}

	entry = KubernetesEntry(KubernetesOptions{
		Client: NewNodeInfoClient(s.URL),
		hackPort: 8802,
	})
	third, err := Join(
		Options{SelfSpec: entry},
		entry,
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
