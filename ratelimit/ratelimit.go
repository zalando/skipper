package ratelimit

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	circularbuffer "github.com/szuecs/rate-limit-buffer"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/swarm"
)

// Type defines the type of the used breaker: consecutive, rate or
// disabled.
type Type int

const (
	// Header is
	Header = "X-Rate-Limit"
	// RetryHeader is name of the header which will be used to indicate how
	// long a client should wait before making a new request
	RetryAfterHeader = "Retry-After"
	// ServiceRatelimitName is the name of the Ratelimit filter, which will be shown in log
	ServiceRatelimitName = "ratelimit"
	// LocalRatelimitName is the name of the LocalRatelimit filter, which will be shown in log
	LocalRatelimitName = "localRatelimit"
	// ClusterRatelimitName is the name of the ClusterRatelimit filter, which will be shown in log
	ClusterRatelimitName = "clusterRatelimit"
	// DisableRatelimitName is the name of the DisableRatelimit, which will be shown in log
	DisableRatelimitName = "disableRatelimit"
)

const (
	// NoRatelimit is not used
	NoRatelimit Type = iota
	// ServiceRatelimit is used to have a simple rate limit for a
	// backend service, which is calculated and measured within
	// each instance
	ServiceRatelimit
	// LocalRatelimit is used to have a simple local rate limit
	// per user for a backend, which is calculated and measured
	// within each instance
	LocalRatelimit
	// ClusterRatelimit is used to calculate a rate limit for a
	// whole skipper fleet, needs swarm to be enabled
	ClusterRatelimit
	// DisableRatelimit is used to disable rate limit
	DisableRatelimit
)

// Lookuper makes it possible to be more flexible for ratelimiting.
type Lookuper interface {
	// Lookup is used to get the string which is used to define
	// how the bucket of a ratelimiter looks like, which is used
	// to decide to ratelimit or not. For example you can use the
	// X-Forwarded-For Header if you want to rate limit based on
	// source ip behind a proxy/loadbalancer or the Authorization
	// Header for request per token or user.
	Lookup(*http.Request) string
}

// SameBucketLookuper implements Lookuper interface and will always
// match to the same bucket.
type SameBucketLookuper struct{}

// NewSameBucketLookuper returns a SameBucketLookuper.
func NewSameBucketLookuper() SameBucketLookuper {
	return SameBucketLookuper{}
}

// Lookup will always return "s" to select the same bucket.
func (SameBucketLookuper) Lookup(*http.Request) string {
	return "s"
}

// XForwardedForLookuper implements Lookuper interface and will
// select a bucket by X-Forwarded-For header or clientIP.
type XForwardedForLookuper struct{}

// NewXForwardedForLookuper returns an empty XForwardedForLookuper
func NewXForwardedForLookuper() XForwardedForLookuper {
	return XForwardedForLookuper{}
}

// Lookup returns the content of the X-Forwarded-For header or the
// clientIP if not set.
func (XForwardedForLookuper) Lookup(req *http.Request) string {
	return net.RemoteHost(req).String()
}

// AuthLookuper implements Lookuper interface and will select a bucket
// by Authorization header.
type AuthLookuper struct{}

// NewAuthLookuper returns an empty AuthLookuper
func NewAuthLookuper() AuthLookuper {
	return AuthLookuper{}
}

// Lookup returns the content of the Authorization header.
func (AuthLookuper) Lookup(req *http.Request) string {
	return req.Header.Get("Authorization")
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
	case ServiceRatelimit:
		return fmt.Sprintf("ratelimit(type=service,max-hits=%d,time-window=%s)", s.MaxHits, s.TimeWindow)
	case LocalRatelimit:
		return fmt.Sprintf("ratelimit(type=local,max-hits=%d,time-window=%s)", s.MaxHits, s.TimeWindow)
	case ClusterRatelimit:
		return fmt.Sprintf("ratelimit(type=cluster,max-hits=%d,time-window=%s)", s.MaxHits, s.TimeWindow)
	default:
		return "non"
	}
}

// TODO(sszuecs): better name
type implementation interface {
	// Allow is used to get a decision if you should allow the call to pass or to ratelimit
	Allow(string) bool

	// Close is used to clean up underlying implementations, if you want to stop a Ratelimiter
	Close()

	// Current returns the oldest timestamp for string
	Current(string) time.Time

	// RetryAfter is used to inform the client how many seconds it should wait
	// before making a new request
	RetryAfter(string) int

	// Delta is used to get the duration until the next call is possible, negative durations allow
	// immediate calls
	Delta(string) time.Duration

	// Resize is used to resize the buffer depending on the number of nodes available
	Resize(string, int)
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

// RetryAfter informs how many seconds to wait for the next request
func (l *Ratelimit) RetryAfter(s string) int {
	if l == nil {
		return 0
	}
	return l.impl.RetryAfter(s)
}

func (l *Ratelimit) Delta(s string) time.Duration {
	return l.impl.Delta(s)
}

func (l *Ratelimit) Resize(s string, i int) {
	l.impl.Resize(s, i)
}

type voidRatelimit struct{}

// Allow always returns true, not ratelimited
func (voidRatelimit) Allow(string) bool          { return true }
func (voidRatelimit) Close()                     {}
func (voidRatelimit) Current(string) time.Time   { return time.Time{} }
func (voidRatelimit) RetryAfter(string) int      { return 0 }
func (voidRatelimit) Delta(string) time.Duration { return -1 * time.Second }
func (voidRatelimit) Resize(string, int)         {}

func newRatelimit(s Settings, sw *swarm.Swarm) *Ratelimit {
	var impl implementation
	switch s.Type {
	case ServiceRatelimit:
		impl = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow)
	case LocalRatelimit:
		impl = circularbuffer.NewClientRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval)
	case ClusterRatelimit:
		logrus.Infof("SWARM: cluster ratelimit found, try to create..")
		if sw != nil {
			impl = NewClusterRateLimiter(s, sw, "foo")
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: no -enable-swarm, falling back to no ratelimit for %q\n", s)
			impl = voidRatelimit{}
		}
	default:
		impl = voidRatelimit{}
	}

	return &Ratelimit{
		settings: s,
		impl:     impl,
	}
}
