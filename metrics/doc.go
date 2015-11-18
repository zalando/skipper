/*
Package metrics implements collection of common performance metrics.

It uses the Go implementation of the Coda Hale metrics library:

https://github.com/dropwizard/metrics

The collected metrics include the total request processing time, the
time of looking up routes, the time spent with processing all filters
and every single filter, the time waiting for the response from the
backend services, and the time spent with forwarding the response to the
client.

Options

To enable metrics, it needs to be initialized with a Listener address.
In this case, Skipper will start an additional http listener, where the
current metrics values can be downloaded.

[TODO]
*/
package metrics
