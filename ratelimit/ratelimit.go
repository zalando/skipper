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
	// LocalRatelimitName is the name of the LocalRatelimit filter, which will be shown in log
	LocalRatelimitName = "localRatelimit"
	// DisableRatelimitName is the name of the DisableRatelimit, which will be shown in log
	DisableRatelimitName = "disableRatelimit"
)

const (
	// NoRatelimit is not used
	NoRatelimit Type = iota
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
	if to.Type == NoRatelimit {
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
		return fmt.Sprintf("ratelimit(type=local,max-hits=%d,time-window=%s)", s.MaxHits, s.TimeWindow)
	default:
		return "non"
	}
}

type implementation interface {
	// Allow is used to get a decision if you should allow the call to pass or to ratelimit
	Allow(string) bool
	// Close is used to clean up underlying implementations, if you want to stop a Ratelimiter
	Close()
}

// Ratelimit is a proxy objects that delegates to implemetations and
// stores settings for the ratelimiter
type Ratelimit struct {
	settings Settings
	ts       time.Time
	impl     implementation
}

// Allow returns true if the s is not ratelimited, false if it is
// ratelimited
func (l *Ratelimit) Allow(s string) bool {
	if l == nil {
		return true
	}
	return l.impl.Allow(s)
}

// Close will stop a cleanup goroutines in underlying implementation.
func (l *Ratelimit) Close() {
	l.impl.Close()
}

type voidRatelimit struct{}

// Allow always returns true, not ratelimited
func (l voidRatelimit) Allow(string) bool {
	return true
}
func (l voidRatelimit) Close() {
}

func newRatelimit(s Settings) *Ratelimit {
	var impl implementation
	switch s.Type {
	case LocalRatelimit:
		impl = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval)
	default:
		impl = voidRatelimit{}
	}

	return &Ratelimit{
		settings: s,
		impl:     impl,
	}
}
