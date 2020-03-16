package fastcgi

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"

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

	handler := gofast.NewFileEndpoint(filename)(gofast.BasicSession)

	return &RoundTripper{
		log:     log,
		client:  client,
		handler: handler,
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
		return nil, fmt.Errorf("gofast: error stream from application process %s", errBuffer.String())
	}

	return rr.Result(), nil
}
