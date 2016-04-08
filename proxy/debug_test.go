package proxy

import "testing"

func TestJson(t *testing.T) {
	// all
	// none
}

func TestBody(t *testing.T) {
	// nil
	// error reader
}

func TestResponseDiff(t *testing.T) {
	// nil
	// status not 0
	// body not nil
}

func TestDebug(t *testing.T) {
	// route not found
	// route id
	// route expression
	// incoming body
	// outgoing body
	// proxy error
	// has response
	// has no response
	// filter panic
}
