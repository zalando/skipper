package ratelimit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	circularbuffer "github.com/szuecs/rate-limit-buffer"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
)

const (
	// Header is
	Header = "X-Rate-Limit"

	// RetryHeader is name of the header which will be used to indicate how
	// long a client should wait before making a new request
	RetryAfterHeader = "Retry-After"

	// Deprecated, use filters.RatelimitName instead
	ServiceRatelimitName = filters.RatelimitName

	// LocalRatelimitName *DEPRECATED*, use ClientRatelimitName instead
	LocalRatelimitName = "localRatelimit"

	// Deprecated, use filters.ClientRatelimitName instead
	ClientRatelimitName = filters.ClientRatelimitName

	// Deprecated, use filters.ClusterRatelimitName instead
	ClusterServiceRatelimitName = filters.ClusterRatelimitName

	// Deprecated, use filters.ClusterClientRatelimitName instead
	ClusterClientRatelimitName = filters.ClusterClientRatelimitName

	// Deprecated, use filters.DisableRatelimitName instead
	DisableRatelimitName = filters.DisableRatelimitName

	// Deprecated, use filters.UnknownRatelimitName instead
	UknownRatelimitName = filters.UnknownRatelimitName

	sameBucket = "s"
)

// RatelimitType defines the type of  the used ratelimit
type RatelimitType int

func (rt *RatelimitType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}

	switch value {
	case "local":
		log.Warning("LocalRatelimit is deprecated, please use ClientRatelimit instead")
		fallthrough
	case "client":
		*rt = ClientRatelimit
	case "service":
		*rt = ServiceRatelimit
	case "clusterClient":
		*rt = ClusterClientRatelimit
	case "clusterService":
		*rt = ClusterServiceRatelimit
	case "disabled":
		*rt = DisableRatelimit
	default:
		return fmt.Errorf("invalid ratelimit type %v (allowed values are: client, service or disabled)", value)
	}

	return nil
}

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
	// swarm to be enabled with -enable-swarm. In case of redis it
	// will not consume more memory.
	// In case of swim based cluster ratelimit, one filter
	// consumes memory calculated by the following formular, where
	// N is the number of individual clients put into the same
	// bucket, M the maximum number of requests allowed, S the
	// number of skipper peers:
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
		return filters.DisableRatelimitName
	case ClientRatelimit:
		return filters.ClientRatelimitName
	case ClusterClientRatelimit:
		return filters.ClusterClientRatelimitName
	case ClusterServiceRatelimit:
		return filters.ClusterRatelimitName
	case LocalRatelimit:
		return LocalRatelimitName
	case ServiceRatelimit:
		return filters.RatelimitName
	default:
		return filters.UnknownRatelimitName

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
	return sameBucket
}

func (SameBucketLookuper) String() string {
	return "SameBucketLookuper"
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

func (XForwardedForLookuper) String() string {
	return "XForwardedForLookuper"
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

func (h HeaderLookuper) String() string {
	return "HeaderLookuper"
}

// Lookupers is a slice of Lookuper, required to get a hashable member
// in the TupleLookuper.
type Lookupers []Lookuper

// TupleLookuper implements Lookuper interface and will select a
// bucket that is defined by all combined Lookupers.
type TupleLookuper struct {
	// pointer is required to be hashable from Registry lookup table
	l *Lookupers
}

// NewTupleLookuper returns TupleLookuper configured to lookup the
// combined result of all given Lookuper
func NewTupleLookuper(args ...Lookuper) TupleLookuper {
	var ls Lookupers = args
	return TupleLookuper{l: &ls}
}

// Lookup returns the combined string of all Lookupers part of the
// tuple
func (t TupleLookuper) Lookup(req *http.Request) string {
	if t.l == nil {
		return ""
	}

	buf := bytes.Buffer{}
	for _, l := range *(t.l) {
		buf.WriteString(l.Lookup(req))
	}
	return buf.String()
}

func (t TupleLookuper) String() string {
	return "TupleLookuper"
}

// RoundRobinLookuper matches one of n buckets selected by round robin algorithm
type RoundRobinLookuper struct {
	// pointer is required to be hashable from Registry lookup table
	c *uint64
	// number of buckets, unchanged after creation
	n uint64
}

// NewRoundRobinLookuper returns a RoundRobinLookuper.
func NewRoundRobinLookuper(n uint64) Lookuper {
	if n == 0 {
		// Avoid division by zero
		log.Warn("NewRoundRobinLookuper called with n=0, defaulting to 1 bucket.")
		n = 1
	}
	return &RoundRobinLookuper{c: new(uint64), n: n}
}

// Lookup will return one of n distinct keys in round robin fashion
func (rrl *RoundRobinLookuper) Lookup(*http.Request) string {
	next := atomic.AddUint64(rrl.c, 1) % rrl.n
	return fmt.Sprintf("RoundRobin%d", next)
}

func (rrl *RoundRobinLookuper) String() string {
	return "RoundRobinLookuper"
}

// Settings configures the chosen rate limiter
type Settings struct {
	// FailClosed allows to to decide what happens on failures to
	// query the ratelimit. For example redis is down, fail open
	// or fail closed. FailClosed set to true will deny the
	// request and set to true will allow the request. Default is
	// to fail open.
	FailClosed bool `yaml:"fail-closed"`

	// Type of the chosen rate limiter
	Type RatelimitType `yaml:"type"`

	// Lookuper to decide which data to use to identify the same
	// bucket (for example how to lookup the client identifier)
	Lookuper Lookuper `yaml:"-"`

	// MaxHits the maximum number of hits for a time duration
	// allowed in the same bucket.
	MaxHits int `yaml:"max-hits"`

	// TimeWindow is the time duration that is valid for hits to
	// be counted in the rate limit.
	TimeWindow time.Duration `yaml:"time-window"`

	// CleanInterval is the duration old data can expire, because
	// need to cleanup data in for example client ratelimits.
	CleanInterval time.Duration `yaml:"-"`

	// Group is a string to group ratelimiters of Type
	// ClusterServiceRatelimit or ClusterClientRatelimit.
	// A ratelimit group considers all hits to the same group as
	// one target.
	Group string `yaml:"group"`
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
		return "unknown"
	}
}

// limiter defines the requirement to be used as a ratelimit implementation.
type limiter interface {
	// Allow is used to get a decision if you should allow the
	// call with context, to pass or to ratelimit
	Allow(context.Context, string) bool

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
// implementations and stores settings for the ratelimiter
type Ratelimit struct {
	settings Settings
	impl     limiter
}

// Allow is used to get a decision if you should allow the call
// with context, e.g. to support OpenTracing.
func (l *Ratelimit) Allow(ctx context.Context, s string) bool {
	if l == nil || l.impl == nil {
		log.Warn("Allow called on nil or uninitialized Ratelimit object")
		return true // Defaulting to fail open
	}

	return l.impl.Allow(ctx, s)
}

// Close will stop any cleanup goroutines in underlying limiter implementation.
func (l *Ratelimit) Close() {
	if l != nil && l.impl != nil {
		l.impl.Close()
	}
}

// RetryAfter informs how many seconds to wait for the next request
func (l *Ratelimit) RetryAfter(s string) int {
	if l == nil || l.impl == nil {
		return 0
	}
	return l.impl.RetryAfter(s)
}

func (l *Ratelimit) Delta(s string) time.Duration {
	if l == nil || l.impl == nil {
		// Return a large negative duration to indicate immediate allow if uninitialized
		return -1 * time.Hour
	}
	return l.impl.Delta(s)
}

func (l *Ratelimit) Resize(s string, i int) {
	if l != nil && l.impl != nil {
		l.impl.Resize(s, i)
	}
}

type voidRatelimit struct{}

func (voidRatelimit) Allow(context.Context, string) bool { return true }
func (voidRatelimit) Close()                             {}
func (voidRatelimit) Oldest(string) time.Time            { return time.Time{} }
func (voidRatelimit) RetryAfter(string) int              { return 0 }
func (voidRatelimit) Delta(string) time.Duration         { return -1 * time.Second }
func (voidRatelimit) Resize(string, int)                 {}

type zeroRatelimit struct{}

const (
	// Delta() and RetryAfter() should return consistent values of type int64 and int respectively.
	//
	// News had just come over,
	// We had five years left to cry in
	zeroDelta time.Duration = 5 * 365 * 24 * time.Hour
	zeroRetry int           = int(zeroDelta / time.Second)
)

func (zeroRatelimit) Allow(context.Context, string) bool { return false }
func (zeroRatelimit) Close()                             {}
func (zeroRatelimit) Oldest(string) time.Time            { return time.Time{} }
func (zeroRatelimit) RetryAfter(string) int              { return zeroRetry }
func (zeroRatelimit) Delta(string) time.Duration         { return zeroDelta }
func (zeroRatelimit) Resize(string, int)                 {}

func newRatelimit(s Settings, sw Swarmer, redisClient *net.RedisClient) *Ratelimit {
	var impl limiter
	if s.MaxHits <= 0 {
		log.Warnf("MaxHits is %d, creating a zeroRatelimit (always deny)", s.MaxHits)
		impl = zeroRatelimit{}
	} else {
		switch s.Type {
		case ServiceRatelimit:
			if s.Lookuper == nil {
				s.Lookuper = NewSameBucketLookuper()
			}
			impl = circularbuffer.NewRateLimiter(s.MaxHits, s.TimeWindow)
		case LocalRatelimit:
			log.Warning("LocalRatelimit is deprecated, please use ClientRatelimit instead")
			fallthrough
		case ClientRatelimit:
			if s.CleanInterval <= 0 {
				s.CleanInterval = s.TimeWindow * 2
				log.Debugf("ClientRatelimit CleanInterval not set, defaulting to %v", s.CleanInterval)
			}
			impl = circularbuffer.NewClientRateLimiter(s.MaxHits, s.TimeWindow, s.CleanInterval)
		case ClusterServiceRatelimit:
			s.CleanInterval = 0
			fallthrough
		case ClusterClientRatelimit:
			impl = newClusterRateLimiter(s, sw, redisClient, s.Group)
		case DisableRatelimit:
			impl = voidRatelimit{}
		default:
			log.Warnf("Unknown ratelimit type %v specified, disabling ratelimit.", s.Type)
			impl = voidRatelimit{}
		}
	}

	if impl == nil {
		log.Error("Failed to initialize limiter implementation, defaulting to voidRatelimit.")
		impl = voidRatelimit{}
	}

	return &Ratelimit{
		settings: s,
		impl:     impl,
	}
}

// Headers generates standard rate limit related HTTP headers.
func Headers(maxHits int, timeWindow time.Duration, retryAfter int) http.Header {
	// Ensure maxHits is non-negative
	if maxHits < 0 {
		maxHits = 0
	}

	h := make(http.Header)
	// Use RateLimit-Limit standard header name
	// https://datatracker.ietf.org/doc/html/draft-polli-ratelimit-headers-10#section-4.1
	h.Set("RateLimit-Limit", strconv.Itoa(maxHits))
	if timeWindow > time.Microsecond { // Avoid division by zero or near-zero; use microseconds for precision check
		limitPerHour := int64(float64(maxHits) * float64(time.Hour) / float64(timeWindow))
		h.Set(Header, strconv.FormatInt(limitPerHour, 10))
	} else {
		// Avoid division by zero if timeWindow is invalid
		h.Set(Header, strconv.Itoa(maxHits)) // Just report maxHits if window is zero
	}

	if retryAfter > 0 {
		// Use RateLimit-Reset standard header name (relative seconds)
		// https://datatracker.ietf.org/doc/html/draft-polli-ratelimit-headers-10#section-4.3
		h.Set("RateLimit-Reset", strconv.Itoa(retryAfter))
		// Keep the non-standard Retry-After for compatibility
		// For HTTP standard Retry-After (RFC 9110 section 10.2.3), it's typically used with 429 or 503
		h.Set(RetryAfterHeader, strconv.Itoa(retryAfter))
	}
	return h
}

// getHashedKey generates a SHA256 hash of the input string for use as a Redis key or similar.
func getHashedKey(clearText string) string {
	h := sha256.Sum256([]byte(clearText))
	return hex.EncodeToString(h[:])
}
