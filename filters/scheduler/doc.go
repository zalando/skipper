// Package scheduler implements filter logic that changes the http
// request scheduling behavior of the proxy.
//
// The proxy has as default an unbounded scheduler that does not limit
// inflight requests. Goroutines with parsed request data consume
// memory. The unbounded handler could spike in memory, if you have
// traffic on a backend that has too big response times. You can check
// the number of goroutines from skipper metrics, if you have this
// problem.
//
// The scheduler filter package has two implementations of bounded
// queue, the lifo and lifoGroup filter. Both lifo filters will, use a
// last in first out queue to handle most requests fast and if skipper
// is in an overrun mode, it will serve some requests fast and some
// will timeout.  This scheduler implementation makes sure that one
// route will not interfere with other routes, if these routes are not
// in the same scheduler group. LifoGroup has a user specified
// scheduler group and lifo will get a per route unique scheduler
// group.
//
// Bounded schedulers were tested in Kubernetes with 3 proxy instances
// with 500m CPU and 500Mi memory resources. The load test was done
// with 500 requests per second to backends with 25 seconds latency
// and a second load test was done in parallel with 50 150 250 .. 1000
// requests per second to backends with no additional latency. For the
// workload without additional latency there was no additional latency
// measurable. The memory was at maximum 350Mi with the bounded
// scheduler. The unbounded scheduler spiked in memory to above 500Mi,
// which caused an out of memory (OOM) kill by the operating system.
//
// Bounded schedulers will respond to requests with server status error
// codes in case of overrun. The scheduler returns HTTP status code:
//
//   - 502, if it cannot get a request from data structure fast enough
//   - 503, if the data structure is full and reached its boundary
package scheduler
