package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

func TestInjectAnnotateFilters(t *testing.T) {
	annotateFilter := func(key, val string) *eskip.Filter {
		return &eskip.Filter{
			Name: filters.AnnotateName,
			Args: []interface{}{key, val},
		}
	}

	existingFilter := &eskip.Filter{Name: "setRequestHeader", Args: []interface{}{"X-Foo", "bar"}}

	t.Run("no keys to inject leaves route unchanged", func(t *testing.T) {
		r := &eskip.Route{Filters: []*eskip.Filter{existingFilter}}
		annotations := map[string]string{"my-annotation": "value"}
		injectAnnotateFilters(annotations, nil, "", r)
		assert.Equal(t, []*eskip.Filter{existingFilter}, r.Filters)
	})

	t.Run("key not present in annotations does not add filter", func(t *testing.T) {
		r := &eskip.Route{Filters: []*eskip.Filter{existingFilter}}
		annotations := map[string]string{"other-key": "value"}
		injectAnnotateFilters(annotations, []string{"my-annotation"}, "", r)
		assert.Equal(t, []*eskip.Filter{existingFilter}, r.Filters)
	})

	t.Run("matching key is prepended as annotate filter", func(t *testing.T) {
		r := &eskip.Route{Filters: []*eskip.Filter{existingFilter}}
		annotations := map[string]string{"my-annotation": "my-value"}
		injectAnnotateFilters(annotations, []string{"my-annotation"}, "", r)
		require := assert.New(t)
		require.Len(r.Filters, 2)
		require.Equal(filters.AnnotateName, r.Filters[0].Name)
		require.Equal([]interface{}{"my-annotation", "my-value"}, r.Filters[0].Args)
		require.Equal(existingFilter, r.Filters[1])
	})

	t.Run("multiple matching keys all prepended", func(t *testing.T) {
		r := &eskip.Route{}
		annotations := map[string]string{
			"key-a": "val-a",
			"key-b": "val-b",
		}
		injectAnnotateFilters(annotations, []string{"key-a", "key-b"}, "", r)
		assert.Len(t, r.Filters, 2)
		names := []string{r.Filters[0].Args[0].(string), r.Filters[1].Args[0].(string)}
		assert.ElementsMatch(t, []string{"key-a", "key-b"}, names)
	})

	t.Run("only present keys are injected when some are missing", func(t *testing.T) {
		r := &eskip.Route{Filters: []*eskip.Filter{existingFilter}}
		annotations := map[string]string{"present": "yes"}
		injectAnnotateFilters(annotations, []string{"present", "absent"}, "", r)
		assert.Len(t, r.Filters, 2)
		assert.Equal(t, filters.AnnotateName, r.Filters[0].Name)
		assert.Equal(t, "present", r.Filters[0].Args[0])
		assert.Equal(t, existingFilter, r.Filters[1])
	})

	t.Run("empty annotations map with keys to inject leaves route unchanged", func(t *testing.T) {
		r := &eskip.Route{Filters: []*eskip.Filter{existingFilter}}
		injectAnnotateFilters(map[string]string{}, []string{"key-a", "key-b"}, "", r)
		assert.Equal(t, []*eskip.Filter{existingFilter}, r.Filters)
	})

	t.Run("nil annotations map with keys to inject leaves route unchanged", func(t *testing.T) {
		r := &eskip.Route{Filters: []*eskip.Filter{existingFilter}}
		injectAnnotateFilters(nil, []string{"key-a"}, "", r)
		assert.Equal(t, []*eskip.Filter{existingFilter}, r.Filters)
	})

	t.Run("route with no existing filters gets annotate filter prepended", func(t *testing.T) {
		r := &eskip.Route{}
		annotations := map[string]string{"my-key": "my-val"}
		injectAnnotateFilters(annotations, []string{"my-key"}, "", r)
		assert.Len(t, r.Filters, 1)
		assert.Equal(t, annotateFilter("my-key", "my-val"), r.Filters[0])
	})

	t.Run("injected annotate filters come before existing filters", func(t *testing.T) {
		existing1 := &eskip.Filter{Name: "filter1"}
		existing2 := &eskip.Filter{Name: "filter2"}
		r := &eskip.Route{Filters: []*eskip.Filter{existing1, existing2}}
		annotations := map[string]string{"k": "v"}
		injectAnnotateFilters(annotations, []string{"k"}, "", r)
		assert.Len(t, r.Filters, 3)
		assert.Equal(t, filters.AnnotateName, r.Filters[0].Name)
		assert.Equal(t, existing1, r.Filters[1])
		assert.Equal(t, existing2, r.Filters[2])
	})

	t.Run("annotation value is correctly propagated to filter args", func(t *testing.T) {
		r := &eskip.Route{}
		annotations := map[string]string{"oidc-tenant": "acme-corp"}
		injectAnnotateFilters(annotations, []string{"oidc-tenant"}, "", r)
		assert.Len(t, r.Filters, 1)
		assert.Equal(t, "oidc-tenant", r.Filters[0].Args[0])
		assert.Equal(t, "acme-corp", r.Filters[0].Args[1])
	})

	t.Run("prefix is prepended to annotate key without separator", func(t *testing.T) {
		r := &eskip.Route{}
		annotations := map[string]string{"oidc/client-id": "my-client"}
		injectAnnotateFilters(annotations, []string{"oidc/client-id"}, "k8s:", r)
		assert.Len(t, r.Filters, 1)
		assert.Equal(t, "k8s:oidc/client-id", r.Filters[0].Args[0])
		assert.Equal(t, "my-client", r.Filters[0].Args[1])
	})

	t.Run("prefix does not affect K8s annotation lookup key", func(t *testing.T) {
		r := &eskip.Route{}
		// annotation stored under "client-id", injected as "prefix:client-id"
		annotations := map[string]string{"client-id": "the-value"}
		injectAnnotateFilters(annotations, []string{"client-id"}, "prefix:", r)
		assert.Len(t, r.Filters, 1)
		assert.Equal(t, "prefix:client-id", r.Filters[0].Args[0])
		assert.Equal(t, "the-value", r.Filters[0].Args[1])
		// also verify the old lookup key is not in the result
		assert.NotEqual(t, "client-id", r.Filters[0].Args[0])
	})

	t.Run("prefix applied to multiple keys independently", func(t *testing.T) {
		r := &eskip.Route{}
		annotations := map[string]string{
			"cid": "c1",
			"sec": "s1",
		}
		injectAnnotateFilters(annotations, []string{"cid", "sec"}, "oidc:", r)
		assert.Len(t, r.Filters, 2)
		keys := []string{r.Filters[0].Args[0].(string), r.Filters[1].Args[0].(string)}
		assert.ElementsMatch(t, []string{"oidc:cid", "oidc:sec"}, keys)
	})

	t.Run("empty prefix behaves identically to no prefix", func(t *testing.T) {
		r1 := &eskip.Route{}
		r2 := &eskip.Route{}
		annotations := map[string]string{"k": "v"}
		injectAnnotateFilters(annotations, []string{"k"}, "", r1)
		injectAnnotateFilters(annotations, []string{"k"}, "", r2)
		assert.Equal(t, r1.Filters, r2.Filters)
		assert.Equal(t, "k", r1.Filters[0].Args[0])
	})

	t.Run("prefix with missing key still adds no filter", func(t *testing.T) {
		r := &eskip.Route{}
		annotations := map[string]string{"other": "v"}
		injectAnnotateFilters(annotations, []string{"missing"}, "pfx:", r)
		assert.Empty(t, r.Filters)
	})
}
