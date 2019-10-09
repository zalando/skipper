package logging

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	logFilter "github.com/zalando/skipper/filters/log"
)

const logOutput = `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`
const logJSONOutput = `{"audit":"","duration":42,"flow-id":"","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`
const logExtendedJSONOutput = `{"audit":"","duration":42,"extra":"extra","flow-id":"","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`

func testRequest() *http.Request {
	r, _ := http.NewRequest("GET", "http://frank@example.com", nil)
	r.RequestURI = "/apache_pb.gif"
	r.RemoteAddr = "127.0.0.1"
	return r
}

func testDate() time.Time {
	l := time.FixedZone("foo", -7*3600)
	return time.Date(2000, 10, 10, 13, 55, 36, 0, l)
}

func testAccessEntry() *AccessEntry {
	return &AccessEntry{
		Request:      testRequest(),
		ResponseSize: 2326,
		StatusCode:   http.StatusTeapot,
		RequestTime:  testDate(),
		Duration:     42 * time.Millisecond}
}

func testAccessLog(t *testing.T, entry *AccessEntry, expectedOutput string, o Options) {
	testAccessLogExtended(t, entry, nil, expectedOutput, o)
}

func testAccessLogExtended(t *testing.T, entry *AccessEntry, additional map[string]interface{}, expectedOutput string, o Options) {
	var buf bytes.Buffer
	o.AccessLogOutput = &buf
	Init(o)
	LogAccess(entry, additional)
	got := buf.String()
	if got != "" {
		got = got[:len(got)-1]
	}

	if got != expectedOutput {
		t.Error("got wrong access log.")
		t.Log("got     :", got)
		t.Log("expected:", expectedOutput)
	}
}

func testAccessLogDefault(t *testing.T, entry *AccessEntry, expectedOutput string) {
	testAccessLog(t, entry, expectedOutput, Options{})
}

func TestAccessLogFormatFull(t *testing.T) {
	testAccessLogDefault(t, testAccessEntry(), logOutput)
}

func TestAccessLogFormatJSON(t *testing.T) {
	testAccessLog(t, testAccessEntry(), logJSONOutput, Options{AccessLogJSONEnabled: true})
}

func TestAccessLogFormatJSONWithAdditionalData(t *testing.T) {
	testAccessLogExtended(t, testAccessEntry(), map[string]interface{}{"extra": "extra"}, logExtendedJSONOutput, Options{AccessLogJSONEnabled: true})
}

func TestAccessLogIgnoresEmptyEntry(t *testing.T) {
	testAccessLogDefault(t, nil, "")
}

func TestNoPanicOnMissingRequest(t *testing.T) {
	entry := testAccessEntry()
	entry.Request = nil
	testAccessLogDefault(
		t,
		entry,
		`- - - [10/Oct/2000:13:55:36 -0700] "- - -" 418 2326 "-" "-" 42 - - -`,
	)
}

func TestUseXForwarded(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3")
	testAccessLogDefault(
		t,
		entry,
		`192.168.3.3 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`,
	)
}

func TestUseXForwardedJSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3")
	testAccessLog(
		t,
		entry,
		`{"audit":"","duration":42,"flow-id":"","host":"192.168.3.3","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
}

func TestStripPortFwd4(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3:6969")
	testAccessLogDefault(
		t,
		entry,
		`192.168.3.3 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`,
	)
}

func TestStripPortFwd4JSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3:6969")
	testAccessLog(
		t, entry,
		`{"audit":"","duration":42,"flow-id":"","host":"192.168.3.3","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
}

func TestStripPortNoFwd4(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.RemoteAddr = "192.168.3.3:6969"
	testAccessLogDefault(
		t,
		entry,
		`192.168.3.3 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`,
	)
}

func TestMissingHostFallback(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.RemoteAddr = ""
	testAccessLogDefault(
		t,
		entry,
		`- - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`,
	)
}

func TestMissingHostFallbackJSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.RemoteAddr = ""
	testAccessLog(
		t,
		entry,
		`{"audit":"","duration":42,"flow-id":"","host":"","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
}

func TestPresentFlowId(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Flow-Id", "sometestflowid")
	testAccessLogDefault(
		t,
		entry,
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com sometestflowid -`)
}

func TestPresentFlowIdJSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Flow-Id", "sometestflowid")
	testAccessLog(
		t,
		entry,
		`{"audit":"","duration":42,"flow-id":"sometestflowid","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
}

func TestPresentAudit(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set(logFilter.UnverifiedAuditHeader, "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0")
	testAccessLogDefault(
		t,
		entry,
		`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - c4ddfe9d-a0d3-4afb-bf26-24b9588731a0`,
	)
}

func TestPresentAuditJSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set(logFilter.UnverifiedAuditHeader, "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0")
	testAccessLog(
		t,
		entry,
		`{"audit":"c4ddfe9d-a0d3-4afb-bf26-24b9588731a0","duration":42,"flow-id":"","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
}

func TestAccessLogStripQuery(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.RequestURI += "?foo=bar"
	testAccessLog(t, entry, logOutput, Options{AccessLogStripQuery: true})
}
