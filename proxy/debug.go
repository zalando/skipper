package proxy

import (
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

	debugResponseMod struct {
		Status *int        `json:"status,omitempty"`
		Header http.Header `json:"header,omitempty"`
	}

	debugDocument struct {
		RouteId         string            `json:"route_id,omitempty"`
		Route           string            `json:"route,omitempty"`
		Incoming        *debugRequest     `json:"incoming,omitempty"`
		Outgoing        *debugRequest     `json:"outgoing,omitempty"`
		ResponseMod     *debugResponseMod `json:"response_mod,omitempty"`
		RequestBody     string            `json:"request_body,omitempty"`
		RequestErr      string            `json:"request_error,omitempty"`
		ResponseModBody string            `json:"response_mod_body,omitempty"`
		ResponseModErr  string            `json:"response_mod_error,omitempty"`
		ProxyError      string            `json:"proxy_error,omitempty"`
		FilterPanics    []string          `json:"filter_panics,omitempty"`
	}
)

type debugInfo struct {
	route        *eskip.Route
	incoming     *http.Request
	outgoing     *http.Request
	response     *http.Response
	err          error
	filterPanics []interface{}
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

func convertDebugInfo(d *debugInfo) debugDocument {
	doc := debugDocument{}
	if d.route != nil {
		doc.RouteId = d.route.Id
		doc.Route = d.route.String()
	}

	var requestBody io.Reader
	if d.incoming == nil {
		log.Error("[debug response] missing incoming request")
	} else {
		doc.Incoming = convertRequest(d.incoming)
		requestBody = d.incoming.Body
	}

	if d.outgoing != nil {
		doc.Outgoing = convertRequest(d.outgoing)

		// if there is an outgoing request, use the body from there
		requestBody = d.outgoing.Body
	}

	if requestBody != nil {
		doc.RequestBody, doc.RequestErr = convertBody(requestBody)
	}

	if d.response != nil {
		if d.response.StatusCode != 0 || len(d.response.Header) != 0 {
			doc.ResponseMod = &debugResponseMod{Header: d.response.Header}
			if d.response.StatusCode != 0 {
				s := d.response.StatusCode
				doc.ResponseMod.Status = &s
			}
		}

		if d.response.Body != nil {
			doc.ResponseModBody, doc.ResponseModErr = convertBody(d.response.Body)
		}
	}

	if d.err != nil {
		doc.ProxyError = d.err.Error()
	}

	for _, fp := range d.filterPanics {
		doc.FilterPanics = append(doc.FilterPanics, fmt.Sprint(fp))
	}

	return doc
}

func dbgResponse(w http.ResponseWriter, d *debugInfo) {
	doc := convertDebugInfo(d)
	enc := json.NewEncoder(w)
	if err := enc.Encode(&doc); err != nil {
		log.Error("[debug response]", err)
	}
}
