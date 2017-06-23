package logging

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

const logOutput = `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "" "" 42 example.com`
const logJSONOutput = `{"duration":42,"host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`

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

func testAccessLog(t *testing.T, entry *AccessEntry, expectedOutput string, accessLogJSONDisabled bool) {
	var buf bytes.Buffer
	Init(Options{AccessLogOutput: &buf, AccessLogJSONDisabled: accessLogJSONDisabled})
	LogAccess(entry)
	got := buf.String()
	if got != "" {
		got = got[:len(got)-1]
	}

	if got != expectedOutput {
		t.Error("got wrong access log.")
		t.Log("expected:", expectedOutput)
		t.Log("got     :", got)
	}
}

func TestAccessLogFormatFull(t *testing.T) {
	testAccessLog(t, testAccessEntry(), logOutput, true)
}

func TestAccessLogFormatJSON(t *testing.T) {
	testAccessLog(t, testAccessEntry(), logJSONOutput, false)
}

func TestAccessLogIgnoresEmptyEntry(t *testing.T) {
	testAccessLog(t, nil, "", true)
}

func TestNoPanicOnMissingRequest(t *testing.T) {
	entry := testAccessEntry()
	entry.Request = nil
	testAccessLog(t, entry, `- - - [10/Oct/2000:13:55:36 -0700] "  " 418 2326 "" "" 42 `, true)
}

func TestUseXForwarded(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3")
	testAccessLog(t, entry, `192.168.3.3 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "" "" 42 example.com`, true)
}

func TestUseXForwardedJSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3")
	testAccessLog(t, entry, `{"duration":42,"host":"192.168.3.3","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`, false)
}

func TestStripPortFwd4(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3:6969")
	testAccessLog(t, entry, `192.168.3.3 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "" "" 42 example.com`, true)
}

func TestStripPortFwd4JSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3:6969")
	testAccessLog(t, entry, `{"duration":42,"host":"192.168.3.3","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`, false)
}

func TestStripPortNoFwd4(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.RemoteAddr = "192.168.3.3:6969"
	testAccessLog(t, entry, `192.168.3.3 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "" "" 42 example.com`, true)
}

func TestMissingHostFallback(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.RemoteAddr = ""
	testAccessLog(t, entry, `- - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "" "" 42 example.com`, true)
}

func TestMissingHostFallbackJSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.RemoteAddr = ""
	testAccessLog(t, entry, `{"duration":42,"host":"-","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`, false)
}
