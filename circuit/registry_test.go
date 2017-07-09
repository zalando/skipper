package circuit

import (
	"math/rand"
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
		r := NewRegistry(Options{})

		b := r.Get(BreakerSettings{Host: "foo"})
		checkNil(t, b)
	})

	t.Run("only default settings", func(t *testing.T) {
		d := createSettings(5)
		r := NewRegistry(Options{Defaults: d})

		b := r.Get(BreakerSettings{Host: "foo"})
		checkWithoutHost(t, b, r.defaults)
	})

	t.Run("only host settings", func(t *testing.T) {
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(Options{HostSettings: []BreakerSettings{h0, h1}})

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
		r := NewRegistry(Options{
			Defaults:     d,
			HostSettings: []BreakerSettings{h0, h1},
		})

		b := r.Get(BreakerSettings{Host: "foo"})
		checkWithHost(t, b, h0)

		b = r.Get(BreakerSettings{Host: "bar"})
		checkWithHost(t, b, h1)

		b = r.Get(BreakerSettings{Host: "baz"})
		checkWithoutHost(t, b, d)
	})

	t.Run("only custom settings", func(t *testing.T) {
		r := NewRegistry(Options{})

		cs := createHostSettings("foo", 15)
		b := r.Get(cs)
		checkWithHost(t, b, cs)
	})

	t.Run("only default settings, with custom", func(t *testing.T) {
		d := createSettings(5)
		r := NewRegistry(Options{Defaults: d})

		cs := createHostSettings("foo", 15)
		b := r.Get(cs)
		checkWithHost(t, b, cs)
	})

	t.Run("only host settings, with custom", func(t *testing.T) {
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(Options{HostSettings: []BreakerSettings{h0, h1}})

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
		r := NewRegistry(Options{
			Defaults:     d,
			HostSettings: []BreakerSettings{h0, h1},
		})

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
		r := NewRegistry(Options{})

		b := r.Get(createDisabledSettings())
		checkNil(t, b)
	})

	t.Run("only default settings, disabled", func(t *testing.T) {
		d := createSettings(5)
		r := NewRegistry(Options{Defaults: d})

		b := r.Get(createDisabledSettings())
		checkNil(t, b)
	})

	t.Run("only host settings, disabled", func(t *testing.T) {
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(Options{HostSettings: []BreakerSettings{h0, h1}})

		b := r.Get(createDisabledSettings())
		checkNil(t, b)
	})

	t.Run("default and host settings, disabled", func(t *testing.T) {
		d := createSettings(5)
		h0 := createHostSettings("foo", 5)
		h1 := createHostSettings("bar", 5)
		r := NewRegistry(Options{
			Defaults:     d,
			HostSettings: []BreakerSettings{h0, h1},
		})

		b := r.Get(createDisabledSettings())
		checkNil(t, b)
	})
}

func TestRegistryEvictIdle(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	options := Options{
		Defaults: BreakerSettings{
			IdleTTL: 15 * time.Millisecond,
		},
		HostSettings: []BreakerSettings{{
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
		}},
	}
	toEvict := options.HostSettings[2]
	r := NewRegistry(options)

	get := func(host string) {
		b := r.Get(BreakerSettings{Host: host})
		if b == nil {
			t.Error("failed to retrieve breaker")
		}
	}

	get("foo")
	get("bar")
	get("baz")

	time.Sleep(2 * options.Defaults.IdleTTL / 3)

	get("foo")
	get("bar")

	time.Sleep(2 * options.Defaults.IdleTTL / 3)

	get("qux")

	if len(r.lookup) != 3 || r.lookup[toEvict] != nil {
		t.Error("failed to evict breaker from lookup")
		return
	}

	current := r.access.first
	for current != nil {
		if current.settings.Host == "baz" {
			t.Error("failed to evict idle breaker")
			return
		}

		current = current.next
	}
}

func TestIndividualIdle(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// create with default and host specific
	//
	// check for both:
	// - fail n - 1
	// - wait idle
	// - fail
	// - stays closed

	const (
		consecutiveFailures = 5
		idleTimeout         = 15 * time.Millisecond
		hostIdleTimeout     = 6 * time.Millisecond
	)

	r := NewRegistry(Options{
		Defaults: BreakerSettings{
			Type:     ConsecutiveFailures,
			Failures: consecutiveFailures,
			IdleTTL:  idleTimeout,
		},
		HostSettings: []BreakerSettings{{
			Host:    "foo",
			IdleTTL: hostIdleTimeout,
		}},
	})

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

func TestRegistryFuzzy(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	const (
		hostCount                = 1200
		customSettingsCount      = 120
		concurrentRequests       = 2048
		requestDurationMean      = 120 * time.Microsecond
		requestDurationDeviation = 60 * time.Microsecond
		idleTTL                  = time.Second
		duration                 = 3 * time.Second
	)

	genHost := func() string {
		const (
			minHostLength = 12
			maxHostLength = 36
		)

		h := make([]byte, minHostLength+rand.Intn(maxHostLength-minHostLength))
		for i := range h {
			h[i] = 'a' + byte(rand.Intn(int('z'+1-'a')))
		}

		return string(h)
	}

	hosts := make([]string, hostCount)
	for i := range hosts {
		hosts[i] = genHost()
	}

	options := Options{Defaults: BreakerSettings{IdleTTL: idleTTL}}

	settings := make(map[string]BreakerSettings)
	for _, h := range hosts {
		s := BreakerSettings{
			Host:     h,
			Type:     ConsecutiveFailures,
			Failures: 5,
			IdleTTL:  idleTTL,
		}
		options.HostSettings = append(options.HostSettings, s)
		settings[h] = s
	}

	r := NewRegistry(options)

	// the first customSettingsCount hosts can have corresponding custom settings
	customSettings := make(map[string]BreakerSettings)
	for _, h := range hosts[:customSettingsCount] {
		s := settings[h]
		s.Failures = 15
		s.IdleTTL = idleTTL
		customSettings[h] = s
	}

	var syncToken struct{}
	sync := make(chan struct{}, 1)
	sync <- syncToken
	synced := func(f func()) {
		t := <-sync
		f()
		sync <- t
	}

	replaceHostSettings := func(settings map[string]BreakerSettings, old, nu string) {
		if s, ok := settings[old]; ok {
			delete(settings, old)
			s.Host = nu
			settings[nu] = s
		}
	}

	replaceHost := func() {
		synced(func() {
			i := rand.Intn(len(hosts))
			old := hosts[i]
			nu := genHost()
			hosts[i] = nu
			replaceHostSettings(settings, old, nu)
			replaceHostSettings(customSettings, old, nu)
		})
	}

	stop := make(chan struct{})

	getSettings := func(useCustom bool) BreakerSettings {
		var s BreakerSettings
		synced(func() {
			if useCustom {
				s = customSettings[hosts[rand.Intn(customSettingsCount)]]
				return
			}

			s = settings[hosts[rand.Intn(hostCount)]]
		})

		return s
	}

	requestDuration := func() time.Duration {
		mean := float64(requestDurationMean)
		deviation := float64(requestDurationDeviation)
		return time.Duration(rand.NormFloat64()*deviation + mean)
	}

	makeRequest := func(useCustom bool) {
		s := getSettings(useCustom)
		b := r.Get(s)
		if b.settings != s {
			t.Error("invalid breaker received")
			t.Log(b.settings, s)
			close(stop)
		}

		time.Sleep(requestDuration())
	}

	runAgent := func() {
		for {
			select {
			case <-stop:
				return
			default:
			}

			// 1% percent chance for getting a host replaced:
			if rand.Intn(100) == 0 {
				replaceHost()
			}

			// 3% percent of the requests is custom:
			makeRequest(rand.Intn(100) < 3)
		}
	}

	time.AfterFunc(duration, func() {
		close(stop)
	})

	for i := 0; i < concurrentRequests; i++ {
		go runAgent()
	}

	<-stop
}
