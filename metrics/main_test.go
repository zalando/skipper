package metrics

import (
	"os"
	"testing"

	"github.com/AlexanderYastrebov/noleak"
	codahale "github.com/rcrowley/go-metrics"
)

func TestMain(m *testing.M) {
	// codahale library has a global arbiter goroutine to update timers and there is no way to stop it,
	// see https://github.com/rcrowley/go-metrics/pull/46
	// Create a dummy timer to start the arbiter goroutine before running tests such that leak detector does not report it.
	_ = codahale.NewTimer()

	os.Exit(noleak.CheckMain(m))
}
