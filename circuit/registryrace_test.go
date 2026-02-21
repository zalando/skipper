package circuit

import (
	"math/rand"
	"testing"
	"time"
)

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

	settings := []BreakerSettings{{IdleTTL: idleTTL}}

	settingsMap := make(map[string]BreakerSettings)
	for _, h := range hosts {
		s := BreakerSettings{
			Host:     h,
			Type:     ConsecutiveFailures,
			Failures: 5,
			IdleTTL:  idleTTL,
		}
		settings = append(settings, s)
		settingsMap[h] = s
	}

	r := NewRegistry(settings...)

	// the first customSettingsCount hosts can have corresponding custom settings
	customSettings := make(map[string]BreakerSettings)
	for _, h := range hosts[:customSettingsCount] {
		s := settingsMap[h]
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
			replaceHostSettings(settingsMap, old, nu)
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

			s = settingsMap[hosts[rand.Intn(hostCount)]]
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

	for range concurrentRequests {
		go runAgent()
	}

	<-stop
}
