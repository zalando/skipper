/* +build !race */

package circuit

import (
	"testing"
	"time"
)

// no checks, used for race detector
func TestRegistry(t *testing.T) {
	createSettings := func(cf int) BreakerSettings {
		return BreakerSettings{
			Type:     ConsecutiveFailures,
			Failures: cf,
			IdleTTL:  DefaultIdleTTL,
		}
	}

	createHostSettings := func(h string, cf int) BreakerSettings {
		s := createSettings(cf)
		s.Host = h
		return s
	}

	createDisabledSettings := func() BreakerSettings {
		return BreakerSettings{Type: BreakerDisabled}
	}

	checkNil := func(t *testing.T, b *Breaker) {
		if b != nil {
			t.Error("unexpected breaker")
		}
	}

	checkNotNil := func(t *testing.T, b *Breaker) {
		if b == nil {
			t.Error("failed to receive a breaker")
		}
	}

	checkSettings := func(t *testing.T, left, right BreakerSettings) {
		if left != right {
			t.Error("failed to receive breaker with the right settings")
			t.Log(left)
			t.Log(right)
		}
	}

	checkWithoutHost := func(t *testing.T, b *Breaker, s BreakerSettings) {
		checkNotNil(t, b)
		sb := b.settings
		sb.Host = ""
		checkSettings(t, sb, s)
	}

	checkWithHost := func(t *testing.T, b *Breaker, s BreakerSettings) {
		checkNotNil(t, b)
		checkSettings(t, b.settings, s)
	}

	t.Run("no settings", func(t *testing.T) {
		r := NewRegistry()

		b := r.Get(BreakerSettings{Host: "foo"})
		checkNil(t, b)
	})

	t.Run("only default settings", func(t *testing.T) {
		d := createSettings(5)
		r := NewRegistry(d)

		b := r.Get(BreakerSettings{Host: "foo"})
		checkWithoutHost(t, b, r.defaults)
	})

	t.Run("only host settings", func(t *testing.T) {
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(h0, h1)

		b := r.Get(BreakerSettings{Host: "foo"})
		checkWithHost(t, b, h0)

		b = r.Get(BreakerSettings{Host: "bar"})
		checkWithHost(t, b, h1)

		b = r.Get(BreakerSettings{Host: "baz"})
		checkNil(t, b)
	})

	t.Run("default and host settings", func(t *testing.T) {
		d := createSettings(5)
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(d, h0, h1)

		b := r.Get(BreakerSettings{Host: "foo"})
		checkWithHost(t, b, h0)

		b = r.Get(BreakerSettings{Host: "bar"})
		checkWithHost(t, b, h1)

		b = r.Get(BreakerSettings{Host: "baz"})
		checkWithoutHost(t, b, d)
	})

	t.Run("only custom settings", func(t *testing.T) {
		r := NewRegistry()

		cs := createHostSettings("foo", 15)
		b := r.Get(cs)
		checkWithHost(t, b, cs)
	})

	t.Run("only default settings, with custom", func(t *testing.T) {
		d := createSettings(5)
		r := NewRegistry(d)

		cs := createHostSettings("foo", 15)
		b := r.Get(cs)
		checkWithHost(t, b, cs)
	})

	t.Run("only host settings, with custom", func(t *testing.T) {
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(h0, h1)

		cs := createHostSettings("foo", 15)
		b := r.Get(cs)
		checkWithHost(t, b, cs)

		cs = createHostSettings("bar", 15)
		b = r.Get(cs)
		checkWithHost(t, b, cs)

		cs = createHostSettings("baz", 15)
		b = r.Get(cs)
		checkWithHost(t, b, cs)
	})

	t.Run("default and host settings, with custom", func(t *testing.T) {
		d := createSettings(5)
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(d, h0, h1)

		cs := createHostSettings("foo", 15)
		b := r.Get(cs)
		checkWithHost(t, b, cs)

		cs = createHostSettings("bar", 15)
		b = r.Get(cs)
		checkWithHost(t, b, cs)

		cs = createHostSettings("baz", 15)
		b = r.Get(cs)
		checkWithHost(t, b, cs)
	})

	t.Run("no settings and disabled", func(t *testing.T) {
		r := NewRegistry()

		b := r.Get(createDisabledSettings())
		checkNil(t, b)
	})

	t.Run("only default settings, disabled", func(t *testing.T) {
		d := createSettings(5)
		r := NewRegistry(d)

		b := r.Get(createDisabledSettings())
		checkNil(t, b)
	})

	t.Run("only host settings, disabled", func(t *testing.T) {
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(h0, h1)

		b := r.Get(createDisabledSettings())
		checkNil(t, b)
	})

	t.Run("default and host settings, disabled", func(t *testing.T) {
		d := createSettings(5)
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(d, h0, h1)

		b := r.Get(createDisabledSettings())
		checkNil(t, b)
	})
}

func TestRegistryEvictIdle(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	settings := []BreakerSettings{{
		IdleTTL: 15 * time.Millisecond,
	}, {
		Host:     "foo",
		Type:     ConsecutiveFailures,
		Failures: 4,
	}, {
		Host:     "bar",
		Type:     ConsecutiveFailures,
		Failures: 5,
	}, {
		Host:     "baz",
		Type:     ConsecutiveFailures,
		Failures: 6,
	}, {
		Host:     "qux",
		Type:     ConsecutiveFailures,
		Failures: 7,
	}}
	toEvict := settings[3]
	r := NewRegistry(settings...)

	get := func(host string) {
		b := r.Get(BreakerSettings{Host: host})
		if b == nil {
			t.Error("failed to retrieve breaker")
		}
	}

	get("foo")
	get("bar")
	get("baz")

	time.Sleep(2 * settings[0].IdleTTL / 3)

	get("foo")
	get("bar")

	time.Sleep(2 * settings[0].IdleTTL / 3)

	get("qux")

	if len(r.lookup) != 3 || r.lookup[toEvict] != nil {
		t.Error("failed to evict breaker from lookup")
		return
	}

	for s := range r.lookup {
		if s.Host == "baz" {
			t.Error("failed to evict idle breaker")
			return
		}
	}
}

func TestIndividualIdle(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	const (
		consecutiveFailures = 5
		idleTimeout         = 15 * time.Millisecond
		hostIdleTimeout     = 6 * time.Millisecond
	)

	r := NewRegistry(
		BreakerSettings{
			Type:     ConsecutiveFailures,
			Failures: consecutiveFailures,
			IdleTTL:  idleTimeout,
		},
		BreakerSettings{
			Host:    "foo",
			IdleTTL: hostIdleTimeout,
		},
	)

	shouldBeClosed := func(t *testing.T, host string) func(bool) {
		b := r.Get(BreakerSettings{Host: host})
		if b == nil {
			t.Error("failed get breaker")
			return nil
		}

		done, ok := b.Allow()
		if !ok {
			t.Error("breaker unexpectedly open")
			return nil
		}

		return done
	}

	fail := func(t *testing.T, host string) {
		done := shouldBeClosed(t, host)
		if done != nil {
			done(false)
		}
	}

	mkfail := func(t *testing.T, host string) func() {
		return func() {
			fail(t, host)
		}
	}

	t.Run("default", func(t *testing.T) {
		times(consecutiveFailures-1, mkfail(t, "bar"))
		time.Sleep(idleTimeout)
		fail(t, "bar")
		shouldBeClosed(t, "bar")
	})

	t.Run("host", func(t *testing.T) {
		times(consecutiveFailures-1, mkfail(t, "foo"))
		time.Sleep(hostIdleTimeout)
		fail(t, "foo")
		shouldBeClosed(t, "foo")
	})
}
