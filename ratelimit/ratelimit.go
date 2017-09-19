package ratelimit

import (
	"fmt"
	"net/http"
	"time"

	circularbuffer "github.com/szuecs/rate-limit-buffer"
)

// Type defines the type of the used breaker: consecutive, rate or
// disabled.
type Type int

const (
	// Header is
	Header = "X-Rate-Limit"
	// LocalRatelimitName is the name of the LocalRatelimit, which will be shown in log
	LocalRatelimitName = "localRatelimit"
	// DisableRatelimitName is the name of the DisableRatelimit, which will be shown in log
	DisableRatelimitName = "disableRatelimit"
)

const (
	// NonRatelimit is not used
	NonRatelimit Type = iota
	// LocalRatelimit is used to have a simple local rate limit,
	// which is calculated and measured within each instance
	LocalRatelimit
	// DisableRatelimit is used to disable rate limit
	DisableRatelimit
)

type Lookuper interface {
	Lookup(*http.Request) string
}

// Settings configures the chosen rate limiter
type Settings struct {
	Type          Type
	Lookuper      Lookuper
	Host          string
	MaxHits       int
	TimeWindow    time.Duration
	CleanInterval time.Duration
}

func (s Settings) Empty() bool {
	return s == Settings{}
}

func (to Settings) mergeSettings(from Settings) Settings {
	if to.Type == NonRatelimit {
		to.Type = from.Type
	}
	if to.MaxHits == 0 {
		to.MaxHits = from.MaxHits
	}
	if to.TimeWindow == 0 {
		to.TimeWindow = from.TimeWindow
	}
	if to.CleanInterval == 0 {
		to.CleanInterval = from.CleanInterval
	}
	return to
}

func (s Settings) String() string {
	switch s.Type {
	case DisableRatelimit:
		return "disable"
	case LocalRatelimit:
		return fmt.Sprintf("ratelimit(type=local,maxHits=%d,timeWindow=%s)", s.MaxHits, s.TimeWindow)
	default:
		return "non"
	}
}

type implementation interface {
	Allow(string) bool
}

type Ratelimit struct {
	settings Settings
	ts       time.Time
	impl     implementation
}

func (l *Ratelimit) Allow(s string) bool {
	if l == nil {
		return true
	}
	return l.impl.Allow(s)
}

type voidRatelimit struct{}

func (l voidRatelimit) Allow(string) bool {
	return true
}

func newRatelimit(s Settings) *Ratelimit {
	var impl implementation
	switch s.Type {
	case LocalRatelimit:
		impl = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval, nil)
	default:
		impl = voidRatelimit{}
	}

	return &Ratelimit{
		settings: s,
		impl:     impl,
	}
}
