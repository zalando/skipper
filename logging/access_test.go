package logging

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	logFilter "github.com/zalando/skipper/filters/log"
)

const logOutput = `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`
const logJSONOutput = `{"audit":"","auth-user":"","duration":42,"flow-id":"","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`
const logExtendedJSONOutput = `{"audit":"","auth-user":"","duration":42,"extra":"extra","flow-id":"","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`

type accessCustomFormatter struct{}
type accessLogContextKey struct{}

func (c accessCustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {

	if entry.Context != nil {
		if traceId, ok := entry.Context.Value(accessLogContextKey{}).(string); ok {
			return []byte(fmt.Sprintf("%s\n", traceId)), nil
		}
	}
	return []byte(fmt.Sprintf("%s\n", entry.Message)), nil
}

func testRequest(params url.Values) *http.Request {
	r, _ := http.NewRequest("GET", "http://frank@example.com", nil)
	r.RequestURI = "/apache_pb.gif"
	r.RemoteAddr = "127.0.0.1"

	if params != nil {
		r.URL.RawQuery = params.Encode()
	}

	return r
}

func testDate() time.Time {
	l := time.FixedZone("foo", -7*3600)
	return time.Date(2000, 10, 10, 13, 55, 36, 0, l)
}

func testAccessEntry() *AccessEntry {
	return &AccessEntry{
		Request:      testRequest(nil),
		ResponseSize: 2326,
		StatusCode:   http.StatusTeapot,
		RequestTime:  testDate(),
		Duration:     42 * time.Millisecond,
		AuthUser:     ""}
}

func testAccessEntryWithQueryParameters(params url.Values) *AccessEntry {
	testAccessEntry := testAccessEntry()
	testAccessEntry.Request = testRequest(params)

	return testAccessEntry
}

func testAccessLog(t *testing.T, entry *AccessEntry, expectedOutput string, o Options) {
	testAccessLogExtended(t, entry, nil, expectedOutput, o)
}

func testAccessLogExtended(t *testing.T, entry *AccessEntry,
	additional map[string]interface{},
	expectedOutput string,
	o Options,
) {
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

func TestAccessLogWithContext(t *testing.T) {
	entry := testAccessEntry()
	traceId := "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0"
	entry.Request = entry.Request.WithContext(context.WithValue(entry.Request.Context(), accessLogContextKey{}, traceId))
	expectedOutput := traceId

	var buf bytes.Buffer
	o := Options{
		AccessLogOutput:    &buf,
		AccessLogFormatter: &accessCustomFormatter{},
	}

	testAccessLog(t, entry, expectedOutput, o)
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

func TestAccessLogFormatJSONWithMaskedQueryParameters(t *testing.T) {
	additional := map[string]interface{}{KeyMaskedQueryParams: map[string]struct{}{"foo": struct{}{}}}

	params := url.Values{}
	params.Add("foo", "bar")
	testAccessLogExtended(t,
		testAccessEntryWithQueryParameters(params),
		additional,
		`{"audit":"","auth-user":"","duration":42,"flow-id":"","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif?foo=5234164152756840025","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
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

func TestUseXForwardedList(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3, 192.168.4.4")
	testAccessLogDefault(
		t,
		entry,
		`192.168.3.3, 192.168.4.4 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`,
	)
}

func TestUseXForwardedListLiteral(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3:80, 192.168.4.4")
	testAccessLogDefault(
		t,
		entry,
		`192.168.3.3:80, 192.168.4.4 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`,
	)
}

func TestUseXForwardedJSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3")
	testAccessLog(
		t,
		entry,
		`{"audit":"","auth-user":"","duration":42,"flow-id":"","host":"192.168.3.3","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
}

func TestPortFwd4(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3:6969")
	testAccessLogDefault(
		t,
		entry,
		`192.168.3.3:6969 - - [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`,
	)
}

func TestPortFwd4JSON(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set("X-Forwarded-For", "192.168.3.3:6969")
	testAccessLog(
		t, entry,
		`{"audit":"","auth-user":"","duration":42,"flow-id":"","host":"192.168.3.3:6969","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
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
		`{"audit":"","auth-user":"","duration":42,"flow-id":"","host":"","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
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
		`{"audit":"","auth-user":"","duration":42,"flow-id":"sometestflowid","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
}

func TestPresentAuthUser(t *testing.T) {
	entry := testAccessEntry()
	entry.AuthUser = "jsmith"
	testAccessLogDefault(
		t,
		entry,
		`127.0.0.1 - jsmith [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.1" 418 2326 "-" "-" 42 example.com - -`)
}

func TestPresentAuthUserJSON(t *testing.T) {
	entry := testAccessEntry()
	entry.AuthUser = "jsmith"
	testAccessLog(
		t,
		entry,
		`{"audit":"","auth-user":"jsmith","duration":42,"flow-id":"","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
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
		`{"audit":"c4ddfe9d-a0d3-4afb-bf26-24b9588731a0","auth-user":"","duration":42,"flow-id":"","host":"127.0.0.1","level":"info","method":"GET","msg":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true},
	)
}

func TestPresentAuditJSONWithCustomFormatter(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.Header.Set(logFilter.UnverifiedAuditHeader, "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0")
	jsonFormatter := &logrus.JSONFormatter{
		DisableTimestamp: true,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyLevel: "my_level",
			logrus.FieldKeyMsg:   "my_message",
		}}
	testAccessLog(
		t,
		entry,
		`{"audit":"c4ddfe9d-a0d3-4afb-bf26-24b9588731a0","auth-user":"","duration":42,"flow-id":"","host":"127.0.0.1","method":"GET","my_level":"info","my_message":"","proto":"HTTP/1.1","referer":"","requested-host":"example.com","response-size":2326,"status":418,"timestamp":"10/Oct/2000:13:55:36 -0700","uri":"/apache_pb.gif","user-agent":""}`,
		Options{AccessLogJSONEnabled: true, AccessLogJsonFormatter: jsonFormatter},
	)
}

func TestAccessLogStripQuery(t *testing.T) {
	entry := testAccessEntry()
	entry.Request.RequestURI += "?foo=bar"
	testAccessLog(t, entry, logOutput, Options{AccessLogStripQuery: true})
}
