package proxy

import "net/http"

type (
	debugRequest struct {
		Method        string      `json:"method"`
		Uri           string      `json:"uri"`
		Proto         string      `json:"proto"`
		Header        http.Header `json:"header"`
		Host          string      `json:"host"`
		RemoteAddress string      `json:"remote_address"`
		Body          string      `json:"body"`
	}

	debugShunt struct {
		Status int         `json:"status"`
		Header http.Header `json:"header"`
		Body   string      `json:"string"`
	}

	filterError struct {
	}

	proxyError struct {
	}

	debugDocument struct {
		RouteId  string       `json:"route_id"`
		Route    string       `json:"route"`
		Incoming debugRequest `json:"incoming"`
		Outgoing *debugRequest `json:"outgoing"`
		Shunt    *debugShunt  `json:"shunt"`
		ProxyError proxyError `json:"proxy_error"`
		FilterErrors []filterError `json:"filter_errors"`
	}
)
