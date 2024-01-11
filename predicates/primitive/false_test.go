package primitive

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

func TestFalseIgnoreArguments(t *testing.T) {
	spec := NewFalse()

	for _, def := range []string{
		`False()`,
		`False(1)`,
		`False("foo")`,
		`False(0, "foo")`,
	} {
		t.Run(def, func(t *testing.T) {
			pp := eskip.MustParsePredicates(def)
			require.Len(t, pp, 1)

			p, err := spec.Create(pp[0].Args)
			assert.NoError(t, err)

			assert.False(t, p.Match(nil))
		})
	}
}
