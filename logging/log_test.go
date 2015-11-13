package logging

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

const logEntry = `127.0.0.1 - - [10/Oct/2000:13:55:36 +0000] "GET /apache_pb.gif HTTP/1.1" 200 2326 "" "" 42`

func TestLogging(t *testing.T) {
	var buf bytes.Buffer

	o := Options{ApplicationLogPrefix: "", ApplicationLogOutput: &buf, AccessLogOutput: &buf}
	Init(o)

	r, _ := http.NewRequest("GET", "http://frank@127.0.0.1", nil)
	r.RequestURI = "/apache_pb.gif"
	r.RemoteAddr = "127.0.0.1"

	entry := &AccessEntry{
		Request:      r,
		ResponseSize: 2326,
		StatusCode:   http.StatusOK,
		RequestTime:  time.Date(2000, 10, 10, 13, 55, 36, 0, time.FixedZone("Test", -7)),
		Duration:     42 * time.Millisecond,
	}
	Access(entry)

	if buf.String() != logEntry {
		t.Errorf("Got wrong log. Expected '%s' but got '%s'", logEntry, buf.String())
	}
}
