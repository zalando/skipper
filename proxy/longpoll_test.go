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
const detectionTimeout = 1200 * time.Millisecond

type (
	initProxy    func(string) (string, func())
	testScenario func(*testing.T, initProxy)
)

type roundTripResponse struct {
	response *http.Response
	err      error
}

type proxyBackendOptions struct {
	readRequest, sendHeader, sendResponse bool
}

func backendNotifyingCanceledRequests(
	o proxyBackendOptions,
	notifyIncoming chan<- <-chan struct{},
) (url string, closeServer func()) {
	quit := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		canceled := make(chan struct{})

		select {
		case notifyIncoming <- canceled:
		case <-quit:
			return
		}

		if o.readRequest {
			buf := make([]byte, 4)
			for {
				_, err := r.Body.Read(buf)
				if err == io.EOF {
					break
				}

				if err != nil {
					close(canceled)
					return
				}
			}
		}

		if o.sendHeader {
			// in practice, this doesn't block
			w.WriteHeader(http.StatusOK)
			w.(interface{ Flush() }).Flush()
		}

		if o.sendResponse {
			for {
				if _, err := w.Write([]byte("foobar")); err != nil {
					close(canceled)
					return
				}
			}
		}

		select {
		case <-r.Context().Done():
			close(canceled)
			return
		case <-quit:
			return
		}
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
	return s.URL, func() {
		s.Close()
		p.close()
	}
}

// func stdlibProxy(backendURL string) (proxyURL string, closeServer func()) {
// 	parsed, err := url.Parse(backendURL)
// 	if err != nil {
// 		panic(err)
// 	}

// 	p := httputil.NewSingleHostReverseProxy(parsed)
// 	s := httptest.NewServer(p)
// 	return s.URL, s.Close
// }

func cancelableRequest(
	method, url string,
	body io.Reader,
	receiveResponse chan<- roundTripResponse,
) (cancel func(), err error) {
	ctx, cancel := stdlibcontext.WithCancel(stdlibcontext.Background())
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		cancel()
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

func expectSuccessfulResponse(t *testing.T, rsp roundTripResponse) {
	if rsp.err == nil {
		return
	}

	t.Errorf("failed to make request: %v", rsp.err)
}

func expectErrorResponse(t *testing.T, rsp roundTripResponse) {
	if rsp.err != nil {
		return
	}

	t.Error("expected to receive an error but did not")
	rsp.response.Body.Close()
}

func testCancelBeforeResponseReceived(t *testing.T, p initProxy) {
	backendReceivedRequest := make(chan (<-chan struct{}))
	url, closeBackend := backendNotifyingCanceledRequests(proxyBackendOptions{}, backendReceivedRequest)

	if p != nil {
		var closeProxy func()
		url, closeProxy = p(url)
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

func testCancelDuringStreamingRequestBody(t *testing.T, p initProxy) {
	backendReceivedRequest := make(chan (<-chan struct{}))
	url, closeBackend := backendNotifyingCanceledRequests(
		proxyBackendOptions{readRequest: true},
		backendReceivedRequest,
	)

	if p != nil {
		var closeProxy func()
		url, closeProxy = p(url)
		cb := closeBackend
		closeBackend = func() {
			cb() // we need to close the backend first
			closeProxy()
		}
	}

	defer closeBackend()

	body, bodyWriter := io.Pipe()
	responseReceived := make(chan roundTripResponse)
	cancelRequest, err := cancelableRequest("POST", url, body, responseReceived)
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

	for range 64 * 1024 {
		if _, err := bodyWriter.Write([]byte("foobar")); err != nil {
			t.Fatal(err)
		}
	}

	cancelRequest()
	select {
	case <-backendDetectedRequestCancel:
		body.Close() // prevent Transport.RoundTrip hang on reading request
		expectErrorResponse(t, <-responseReceived)
	case <-time.After(detectionTimeout):
		t.Error("failed to detect canceled request on time")

		select {
		case rsp := <-responseReceived:
			expectErrorResponse(t, rsp)
		case <-time.After(detectionTimeout):
			t.Error("timeout")
		}
	}
}

func testCancelAfterResponseReceived(t *testing.T, p initProxy) {
	backendReceivedRequest := make(chan (<-chan struct{}))
	url, closeBackend := backendNotifyingCanceledRequests(
		proxyBackendOptions{sendHeader: true},
		backendReceivedRequest,
	)

	if p != nil {
		var closeProxy func()
		url, closeProxy = p(url)
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

	var rsp roundTripResponse
	select {
	case rsp = <-responseReceived:
		expectSuccessfulResponse(t, rsp)
	case <-time.After(detectionTimeout):
		t.Error("client failed to receive the response on time")
	}

	cancelRequest()
	select {
	case <-backendDetectedRequestCancel:
	case <-time.After(detectionTimeout):
		t.Error("failed to detect canceled request on time")
	}
}

func testCancelDuringStreamingResponseBody(t *testing.T, p initProxy) {
	backendReceivedRequest := make(chan (<-chan struct{}))
	url, closeBackend := backendNotifyingCanceledRequests(
		proxyBackendOptions{sendResponse: true},
		backendReceivedRequest,
	)

	if p != nil {
		var closeProxy func()
		url, closeProxy = p(url)
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

	var rsp roundTripResponse
	select {
	case rsp = <-responseReceived:
		expectSuccessfulResponse(t, rsp)
	case <-time.After(detectionTimeout):
		t.Error("client failed to receive the response on time")
	}

	// rsp.response might be nil
	if t.Failed() {
		return
	}

	defer rsp.response.Body.Close()
	buf := make([]byte, 4)
	for range 64 * 1024 {
		if _, err := rsp.response.Body.Read(buf); err != nil {
			t.Fatal(err)
		}
	}

	cancelRequest()
	select {
	case <-backendDetectedRequestCancel:
	case <-time.After(detectionTimeout):
		t.Error("failed to detect canceled request on time")
	}
}

func TestNotifyBackendOnClosedClient(t *testing.T) {
	scenarios := map[string]testScenario{
		"before response received":       testCancelBeforeResponseReceived,
		"during streaming request body":  testCancelDuringStreamingRequestBody,
		"after response received":        testCancelAfterResponseReceived,
		"during streaming response body": testCancelDuringStreamingResponseBody,
	}

	proxyVariants := map[string]initProxy{
		"without proxy": nil,
		// "alternative proxy": stdlibProxy, // for comparison, fails on some of the tests
		"proxy": proxyForBackend,
	}

	for scenarioName, scenario := range scenarios {
		t.Run(scenarioName, func(t *testing.T) {
			for variantName, proxyVariant := range proxyVariants {
				t.Run(variantName, func(t *testing.T) {
					scenario(t, proxyVariant)
				})
			}
		})
	}
}
