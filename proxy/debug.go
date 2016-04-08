package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"io"
	"io/ioutil"
	"net/http"
)

type (
	debugRequest struct {
		Method        string      `json:"method"`
		Uri           string      `json:"uri"`
		Proto         string      `json:"proto"`
		Header        http.Header `json:"header,omitempty"`
		Host          string      `json:"host,omitempty"`
		RemoteAddress string      `json:"remote_address,omitempty"`
	}

	debugResponseDiff struct {
		// todo: make this a pointer and omit empty
		Status int         `json:"status"`
		Header http.Header `json:"header,omitempty"`
	}

	debugDocument struct {
		RouteId  string        `json:"route_id,omitempty"`
		Route    string        `json:"route,omitempty"`
		Incoming debugRequest  `json:"incoming"`
		Outgoing *debugRequest `json:"outgoing,omitempty"`
		// todo: give the response a better name
		ResponseDiff     *debugResponseDiff `json:"response_diff,omitempty"`
		RequestBody      string             `json:"request_body,omitempty"`
		RequestErr       string             `json:"request_error,omitempty"`
		ResponseDiffBody string             `json:"response_diff_body,omitempty"`
		ResponseDiffErr  string             `json:"response_diff_error,omitempty"`
		ProxyError       string             `json:"proxy_error,omitempty"`
		FilterPanics     []string           `json:"filter_panics,omitempty"`
	}
)

type debugInfo struct {
	route         *eskip.Route
	incoming      *http.Request
	outgoing      *http.Request
	response      *http.Response
	err           error
	errStatusCode int
	filterPanics  []interface{}
}

func convertRequest(r *http.Request) *debugRequest {
	return &debugRequest{
		Method:        r.Method,
		Uri:           r.RequestURI,
		Proto:         r.Proto,
		Header:        r.Header,
		Host:          r.Host,
		RemoteAddress: r.RemoteAddr}
}

func convertBody(body io.Reader) (string, string) {
	if body == nil {
		return "", ""
	}

	b, err := ioutil.ReadAll(body)
	out := string(b)

	var errstr string
	if err == nil {
		errstr = ""
	} else {
		errstr = err.Error()
	}

	return out, errstr
}

func hasResponse(r *http.Response) bool {
	return r != nil && (r.StatusCode != 0 || r.Body != nil)
}

func dbgResponse(w http.ResponseWriter, d *debugInfo) {
	doc := debugDocument{Incoming: *convertRequest(d.incoming)}
	response := d.response
	if d.route == nil {
		response = &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     http.StatusText(http.StatusNotFound),
			Body:       bodyBuffer{bytes.NewBuffer(nil)}}
	} else {
		doc.RouteId = d.route.Id
		doc.Route = d.route.String()
	}

	requestBody := d.incoming.Body
	if d.outgoing != nil {
		doc.Outgoing = convertRequest(d.outgoing)
		requestBody = d.outgoing.Body
	}

	doc.RequestBody, doc.RequestErr = convertBody(requestBody)

	if d.err != nil {
		doc.ProxyError = d.err.Error()
		if response == nil {
			response = &http.Response{}
		}

		response.StatusCode = d.errStatusCode
	}

	if hasResponse(response) {
		doc.ResponseDiff = &debugResponseDiff{
			Status: response.StatusCode,
			Header: response.Header}
		doc.ResponseDiffBody, doc.ResponseDiffErr = convertBody(response.Body)
	}

	for _, fp := range d.filterPanics {
		doc.FilterPanics = append(doc.FilterPanics, fmt.Sprint(fp))
	}

	enc := json.NewEncoder(w)
	err := enc.Encode(&doc)
	if err != nil {
		log.Error("[debug response]", err)
	}
}
