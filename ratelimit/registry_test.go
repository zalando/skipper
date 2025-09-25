package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewRegistryBasic(t *testing.T) {
	// Test basic registry creation - just verify the existing test function
	registry := NewSwarmRegistry(nil, nil, Settings{})
	assert.NotNil(t, registry, "Registry should not be nil")
	registry.Close()
}

// no checks, used for race detector
func TestRegistry(t *testing.T) {
	createSettings := func(maxHits int) Settings {
		return Settings{
			Type:          LocalRatelimit,
			MaxHits:       maxHits,
			TimeWindow:    1 * time.Second,
			CleanInterval: 5 * time.Second,
		}
	}

	checkNil := func(t *testing.T, rl *Ratelimit) {
		if rl != nil {
			t.Error("unexpected ratelimit")
		}
	}

	checkNotNil := func(t *testing.T, rl *Ratelimit) {
		if rl == nil {
			t.Error("failed to receive a ratelimit")
		}
	}

	t.Run("no settings", func(t *testing.T) {
		r := NewSwarmRegistry(nil, nil, Settings{})
		defer r.Close()

		rl := r.Get(Settings{})
		checkNil(t, rl)
	})
	t.Run("with settings", func(t *testing.T) {
		s := createSettings(3)
		r := NewSwarmRegistry(nil, nil, s)
		defer r.Close()

		rl := r.Get(s)
		checkNotNil(t, rl)
	})
}
