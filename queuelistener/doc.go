// Package queuelistener implements a net.Listener interface such that
// we can first limit the concurrent number of connections to skipper
// and second use a different queue algorithm than FIFO. Current
// implementation is a LIFO queue to have a good p50 even if skipper
// proxy suffers of connection overload.
//
// reference https://github.com/golang/go/issues/35407
package queuelistener
