package ratelimit

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-redis/redis"
	circularbuffer "github.com/szuecs/rate-limit-buffer"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/swarm"
)

const (
	// Header is
	Header = "X-Rate-Limit"

	// RetryHeader is name of the header which will be used to indicate how
	// long a client should wait before making a new request
	RetryAfterHeader = "Retry-After"

	// ServiceRatelimitName is the name of the Ratelimit filter, which will be shown in log
	ServiceRatelimitName = "ratelimit"

	// LocalRatelimitName *DEPRECATED*, use ClientRatelimitName instead
	LocalRatelimitName = "localRatelimit"

	// ClientRatelimitName is the name of the ClientRatelimit filter, which will be shown in log
	ClientRatelimitName = "clientRatelimit"

	// ClusterServiceRatelimitName is the name of the ClusterServiceRatelimit filter, which will be shown in log
	ClusterServiceRatelimitName = "clusterRatelimit"

	// ClusterClientRatelimitName is the name of the ClusterClientRatelimit filter, which will be shown in log
	ClusterClientRatelimitName = "clusterClientRatelimit"

	// DisableRatelimitName is the name of the DisableRatelimit, which will be shown in log
	DisableRatelimitName = "disableRatelimit"

	// UknownRatelimitName is to print unknown ratelimit settings in error messages
	UknownRatelimitName = "unknownRatelimit"
)

// RatelimitType defines the type of  the used ratelimit
type RatelimitType int

const (
	// NoRatelimit is not used
	NoRatelimit RatelimitType = iota

	// ServiceRatelimit is used to have a simple rate limit for a
	// backend service, which is calculated and measured within
	// each instance
	ServiceRatelimit

	// LocalRatelimit *DEPRECATED* will be replaced by ClientRatelimit
	LocalRatelimit

	// ClientRatelimit is used to have a simple local rate limit
	// per user for a backend, which is calculated and measured
	// within each instance. One filter consumes memory calculated
	// by the following formular, where N is the number of
	// individual clients put into the same bucket, M the maximum
	// number of requests allowed:
	//
	//    memory = N * M * 15 byte
	//
	// For example /login protection 100.000 attacker, 10 requests
	// for 1 hour will use roughly 14.6 MB.
	ClientRatelimit

	// ClusterServiceRatelimit is used to calculate a rate limit
	// for a whole skipper fleet for a backend service, needs
	// swarm to be enabled with -enable-swarm.
	ClusterServiceRatelimit

	// ClusterClientRatelimit is used to calculate a rate limit
	// for a whole skipper fleet per user for a backend, needs
	// swarm to be enabled with -enable-swarm.
	// One filter consumes memory calculated
	// by the following formular, where N is the number of
	// individual clients put into the same bucket, M the maximum
	// number of requests allowed, S the number of skipper peers:
	//
	//    memory = N * M * 15 + S * len(peername)
	//
	// For example /login protection 100.000 attacker, 10 requests
	// for 1 hour, 100 skipper peers with each a name of 8
	// characters will use roughly 14.7 MB.
	ClusterClientRatelimit

	// DisableRatelimit is used to disable rate limit
	DisableRatelimit
)

func (rt RatelimitType) String() string {
	switch rt {
	case DisableRatelimit:
		return DisableRatelimitName
	case ClientRatelimit:
		return ClientRatelimitName
	case ClusterClientRatelimit:
		return ClusterClientRatelimitName
	case ClusterServiceRatelimit:
		return ClusterServiceRatelimitName
	case LocalRatelimit:
		return LocalRatelimitName
	case ServiceRatelimit:
		return ServiceRatelimitName
	default:
		return UknownRatelimitName

	}

}

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

// HeaderLookuper implements Lookuper interface and will select a bucket
// by Authorization header.
type HeaderLookuper struct {
	key string
}

// NewHeaderLookuper returns HeaderLookuper configured to lookup header named k
func NewHeaderLookuper(k string) HeaderLookuper {
	return HeaderLookuper{key: k}
}

// Lookup returns the content of the Authorization header.
func (h HeaderLookuper) Lookup(req *http.Request) string {
	return req.Header.Get(h.key)
}

// Settings configures the chosen rate limiter
type Settings struct {
	// Type of the chosen rate limiter
	Type RatelimitType

	// Lookuper to decide which data to use to identify the same
	// bucket (for example how to lookup the client identifier)
	Lookuper Lookuper

	// MaxHits the maximum number of hits for a time duration
	// allowed in the same bucket.
	MaxHits int

	// TimeWindow is the time duration that is valid for hits to
	// be counted in the rate limit.
	TimeWindow time.Duration

	// CleanInterval is the duration old data can expire, because
	// need to cleanup data in for example client ratelimits.
	CleanInterval time.Duration

	// Group is a string to group ratelimiters of Type
	// ClusterServiceRatelimit or ClusterClientRatelimit.
	// A ratelimit group considers all hits to the same group as
	// one target.
	Group string
}

func (s Settings) Empty() bool {
	return s == Settings{}
}

func (s Settings) String() string {
	switch s.Type {
	case DisableRatelimit:
		return "disable"
	case ServiceRatelimit:
		return fmt.Sprintf("ratelimit(type=service,max-hits=%d,time-window=%s)", s.MaxHits, s.TimeWindow)
	case LocalRatelimit:
		fallthrough
	case ClientRatelimit:
		return fmt.Sprintf("ratelimit(type=client,max-hits=%d,time-window=%s)", s.MaxHits, s.TimeWindow)
	case ClusterServiceRatelimit:
		return fmt.Sprintf("ratelimit(type=clusterService,max-hits=%d,time-window=%s,group=%s)", s.MaxHits, s.TimeWindow, s.Group)
	case ClusterClientRatelimit:
		return fmt.Sprintf("ratelimit(type=clusterClient,max-hits=%d,time-window=%s,group=%s)", s.MaxHits, s.TimeWindow, s.Group)
	default:
		return "non"
	}
}

// limiter defines the requirement to be used as a ratelimit implmentation.
type limiter interface {
	// Allow is used to get a decision if you should allow the
	// call to pass or to ratelimit
	Allow(string) bool

	// Close is used to clean up underlying limiter
	// implementations, if you want to stop a Ratelimiter
	Close()

	// Delta is used to get the duration until the next call is
	// possible, negative durations allow immediate calls
	Delta(string) time.Duration

	// Oldest returns the oldest timestamp for string
	Oldest(string) time.Time

	// Resize is used to resize the buffer depending on the number
	// of nodes available
	Resize(string, int)

	// RetryAfter is used to inform the client how many seconds it
	// should wait before making a new request
	RetryAfter(string) int
}

// Ratelimit is a proxy object that delegates to limiter
// implemetations and stores settings for the ratelimiter
type Ratelimit struct {
	settings Settings
	impl     limiter
}

// Allow returns true if the s is not ratelimited, false if it is
// ratelimited
func (l *Ratelimit) Allow(s string) bool {
	if l == nil {
		return true
	}
	return l.impl.Allow(s)
}

// Close will stop any cleanup goroutines in underlying limiter implementation.
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

func (voidRatelimit) Allow(string) bool          { return true }
func (voidRatelimit) Close()                     {}
func (voidRatelimit) Oldest(string) time.Time    { return time.Time{} }
func (voidRatelimit) RetryAfter(string) int      { return 0 }
func (voidRatelimit) Delta(string) time.Duration { return -1 * time.Second }
func (voidRatelimit) Resize(string, int)         {}

func newRatelimit(s Settings, sw Swarmer, so *swarm.Options, redisRing *redis.Ring) *Ratelimit {
	var impl limiter
	switch s.Type {
	case ServiceRatelimit:
		impl = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow)
	case LocalRatelimit:
		fallthrough
	case ClientRatelimit:
		impl = circularbuffer.NewClientRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval)
	case ClusterServiceRatelimit:
		s.CleanInterval = 0
		fallthrough
	case ClusterClientRatelimit:
		impl = newClusterRateLimiter(s, sw, so, redisRing, s.Group)
	default:
		impl = voidRatelimit{}
	}

	return &Ratelimit{
		settings: s,
		impl:     impl,
	}
}
