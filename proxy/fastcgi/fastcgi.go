package fastcgi

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/yookoala/gofast"
	"github.com/zalando/skipper/logging"
)

type RoundTripper struct {
	log     logging.Logger
	client  gofast.Client
	handler gofast.SessionHandler
}

func NewRoundTripper(log logging.Logger, addr, filename string) (*RoundTripper, error) {
	connFactory := gofast.SimpleConnFactory("tcp", addr)

	client, err := gofast.SimpleClientFactory(connFactory, 0)()
	if err != nil {
		return nil, fmt.Errorf("gofast: failed creating client: %w", err)
	}

	chain := gofast.Chain(
		gofast.BasicParamsMap,
		gofast.MapHeader,
		gofast.MapEndpoint(filename),
		func(handler gofast.SessionHandler) gofast.SessionHandler {
			return func(client gofast.Client, req *gofast.Request) (*gofast.ResponsePipe, error) {
				req.Params["HTTP_HOST"] = req.Params["SERVER_NAME"]
				req.Params["SERVER_SOFTWARE"] = "Skipper"

				// Gofast sets this param to `fastcgi` which is not what the backend will expect.
				delete(req.Params, "REQUEST_SCHEME")

				return handler(client, req)
			}
		},
	)

	return &RoundTripper{
		log:     log,
		client:  client,
		handler: chain(gofast.BasicSession),
	}, nil
}

func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	defer func() {
		if rt.client == nil {
			return
		}

		if err := rt.client.Close(); err != nil {
			rt.log.Errorf("gofast: error closing client: %s", err.Error())
		}
	}()

	resp, err := rt.handler(rt.client, gofast.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("gofast: failed to process request: %w", err)
	}

	rr := httptest.NewRecorder()

	errBuffer := new(bytes.Buffer)
	resp.WriteTo(rr, errBuffer)

	if errBuffer.Len() > 0 {
		if strings.Contains(errBuffer.String(), "Primary script unknown") {
			body := "Not Found"
			return &http.Response{
				Status:        "404 Not Found",
				StatusCode:    404,
				Body:          ioutil.NopCloser(bytes.NewBufferString(body)),
				ContentLength: int64(len(body)),
				Request:       req,
				Header:        make(http.Header),
			}, nil
		} else {
			return nil, fmt.Errorf("gofast: error stream from application process %s", errBuffer.String())
		}
	}

	return rr.Result(), nil
}
