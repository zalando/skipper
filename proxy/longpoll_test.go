package proxy

import (
	stdlibcontext "context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// setting it to a high value, for CI context
const detectionTimeout = 120 * time.Millisecond

type roundTripResponse struct {
	response *http.Response
	err      error
}

func backendNotifyingCanceledRequests(notifyIncoming chan<- <-chan struct{}) (url string, closeServer func()) {
	quit := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		canceled := make(chan struct{})

		select {
		case notifyIncoming <- canceled:
		case <-quit:
			return
		}

		select {
		case <-r.Context().Done():
		case <-quit:
			return
		}

		close(canceled)
	}))

	url = s.URL
	closeServer = func() {
		close(quit)
		s.Close()
	}

	return
}

func proxyForBackend(backendURL string) (url string, closeServer func()) {
	route := fmt.Sprintf(`* -> "%s"`, backendURL)
	p, err := newTestProxy(route, FlagsNone)
	if err != nil {
		panic(err)
	}

	s := httptest.NewServer(p.proxy)
	return s.URL, s.Close
}

func cancelableRequest(
	method, url string,
	body io.Reader,
	receiveResponse chan<- roundTripResponse,
) (cancel func(), err error) {
	ctx, cancel := stdlibcontext.WithCancel(stdlibcontext.Background())
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	go func() {
		rsp, err := (&http.Transport{}).RoundTrip(req)
		receiveResponse <- roundTripResponse{response: rsp, err: err}
	}()

	return cancel, nil
}

func unexpectedResponse(t *testing.T, rsp roundTripResponse) {
	t.Error("we should not have received a response")
	if rsp.err != nil {
		t.Log(rsp.err)
	} else {
		rsp.response.Body.Close()
	}
}

func expectErrorResponse(t *testing.T, rsp roundTripResponse) {
	if rsp.err != nil {
		return
	}

	t.Error("expected to receive an error but did not")
	rsp.response.Body.Close()
}

func testCancelBeforeResponseReceived(t *testing.T, withProxy bool) {
	backendReceivedRequest := make(chan (<-chan struct{}))
	url, closeBackend := backendNotifyingCanceledRequests(backendReceivedRequest)

	if withProxy {
		var closeProxy func()
		url, closeProxy = proxyForBackend(url)
		cb := closeBackend
		closeBackend = func() {
			cb() // we need to close the backend first
			closeProxy()
		}
	}

	defer closeBackend()

	responseReceived := make(chan roundTripResponse)
	cancelRequest, err := cancelableRequest("GET", url, nil, responseReceived)
	if err != nil {
		t.Fatal(err)
	}

	var backendDetectedRequestCancel <-chan struct{}
	select {
	case backendDetectedRequestCancel = <-backendReceivedRequest:
	case rsp := <-responseReceived:
		unexpectedResponse(t, rsp)
	case <-time.After(detectionTimeout):
		t.Error("backend failed to receive the request on time")
	}

	cancelRequest()
	select {
	case <-backendDetectedRequestCancel:
	case <-time.After(detectionTimeout):
		t.Error("failed to detect canceled request on time")
	}

	select {
	case rsp := <-responseReceived:
		expectErrorResponse(t, rsp)
	case <-time.After(detectionTimeout):
		t.Error("timeout")
	}
}

// test cases:
// - test with and without proxy
// - before response received
// - after response received
// - check with opentracing, it seems to be a mess
// - check for upgrades
// - full tests for the other direction
// - what happens on different errors, what can cause errors returned by do()

func TestNotifyBackendOnClosedClient(t *testing.T) {
	t.Run("before response received", func(t *testing.T) {
		t.Run("without proxy", func(t *testing.T) {
			testCancelBeforeResponseReceived(t, false)
		})

		t.Run("with proxy", func(t *testing.T) {
			testCancelBeforeResponseReceived(t, true)
		})
	})
}
