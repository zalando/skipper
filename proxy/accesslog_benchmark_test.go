package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zalando/skipper/logging"
)

const loggedAccessLocalResponse = "Hello world!"

type loggedAccessBenchmarkCase struct {
	name         string
	route        string
	target       string
	host         string
	status       int
	responseBody string
	location     string
	userAgent    string
	accept       string
	localBackend bool
}

func BenchmarkAccessLogWithOutput(b *testing.B) {
	for _, tc := range []loggedAccessBenchmarkCase{
		{
			name:         "inline_status200_10B",
			route:        `inline: Path("/hello") -> status(200) -> inlineContent("some bytes") -> <shunt>`,
			target:       "/hello",
			host:         "www.example.org",
			status:       http.StatusOK,
			responseBody: "some bytes",
		},
		{
			name:         "inline_status200_enabled_10B",
			route:        `inline: Path("/hello") -> enableAccessLog(1, 200, 3) -> status(200) -> inlineContent("some bytes") -> <shunt>`,
			target:       "/hello",
			host:         "www.example.org",
			status:       http.StatusOK,
			responseBody: "some bytes",
		},
		{
			name:         "inline_status418_28B",
			route:        `inline: * -> status(418) -> inlineContent("Would you like a cup of tea?") -> <shunt>`,
			target:       "/",
			host:         "www.example.org",
			status:       http.StatusTeapot,
			responseBody: "Would you like a cup of tea?",
		},
		{
			name:         "query_setQuery_local12B",
			target:       "/",
			host:         "localhost:8080",
			status:       http.StatusOK,
			responseBody: loggedAccessLocalResponse,
			userAgent:    "curl/7.49.0",
			accept:       "*/*",
			localBackend: true,
		},
		{
			name:      "redirect_status308",
			route:     `redirect: * -> redirectTo(308, "http://127.0.0.1:9999") -> <shunt>`,
			target:    "/foo",
			host:      "localhost:8080",
			status:    http.StatusPermanentRedirect,
			location:  "http://127.0.0.1:9999/foo",
			userAgent: "curl/7.49.0",
			accept:    "*/*",
		},
		{
			name:      "redirect_modPath_status308",
			route:     `redirect: * -> modPath("/", "/my/new/base/") -> redirectTo(308, "http://127.0.0.1:9999") -> <shunt>`,
			target:    "/foo",
			host:      "localhost:8080",
			status:    http.StatusPermanentRedirect,
			location:  "http://127.0.0.1:9999/my/new/base/foo",
			userAgent: "curl/7.49.0",
			accept:    "*/*",
		},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			benchmarkAccessLogWithOutput(b, tc)
		})
	}
}

func benchmarkAccessLogWithOutput(b *testing.B, tc loggedAccessBenchmarkCase) {
	b.Helper()

	route := tc.route
	var backend *httptest.Server
	if tc.localBackend {
		backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Length", "12")
			w.Header().Set("Server", "Skipper")
			_, _ = io.WriteString(w, loggedAccessLocalResponse)
		}))
		defer backend.Close()
		route = fmt.Sprintf(`forward: * -> setQuery("lang", "pt") -> %q`, backend.URL)
	}

	logFile, err := os.CreateTemp(b.TempDir(), "access-log-*.log")
	if err != nil {
		b.Fatal(err)
	}
	defer logFile.Close()
	info, err := logFile.Stat()
	if err != nil || !info.Mode().IsRegular() {
		b.Fatalf("access log output is not a regular file: %v", err)
	}

	accessLogger := logging.NewAccessLogger(logging.Options{AccessLogOutput: logFile})
	tp, err := newTestProxyWithParams(route, Params{
		AccessLogDisabled: false,
		AccessLogger:      accessLogger,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer tp.close()

	request := httptest.NewRequest(http.MethodGet, "http://"+tc.host+tc.target, nil)
	request.RequestURI = tc.target
	if tc.userAgent != "" {
		request.Header.Set("User-Agent", tc.userAgent)
	}
	if tc.accept != "" {
		request.Header.Set("Accept", tc.accept)
	}
	originalPath := request.URL.Path
	originalQuery := request.URL.RawQuery

	if tc.localBackend {
		// Populate the proxy transport's persistent loopback connection before timing.
		warmResponse := httptest.NewRecorder()
		tp.proxy.ServeHTTP(warmResponse, request)
		assertLoggedAccessBenchmarkResponse(b, warmResponse, tc)
		if err := logFile.Truncate(0); err != nil {
			b.Fatal(err)
		}
		if _, err := logFile.Seek(0, io.SeekStart); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	var lastResponse *httptest.ResponseRecorder
	for b.Loop() {
		request.URL.Path = originalPath
		request.URL.RawQuery = originalQuery
		request.RequestURI = tc.target
		lastResponse = httptest.NewRecorder()
		tp.proxy.ServeHTTP(lastResponse, request)
	}
	b.StopTimer()

	if lastResponse == nil {
		b.Fatal("benchmark did not serve a request")
	}
	assertLoggedAccessBenchmarkResponse(b, lastResponse, tc)
	verifyLoggedAccessBenchmarkOutput(b, logFile, tc)
}

func assertLoggedAccessBenchmarkResponse(b *testing.B, response *httptest.ResponseRecorder, tc loggedAccessBenchmarkCase) {
	b.Helper()
	if response.Code != tc.status {
		b.Fatalf("response status = %d, want %d", response.Code, tc.status)
	}
	if got := response.Body.String(); got != tc.responseBody {
		b.Fatalf("response body = %q, want %q", got, tc.responseBody)
	}
	if tc.location != "" {
		if got := response.Header().Get("Location"); got != tc.location {
			b.Fatalf("Location = %q, want %q", got, tc.location)
		}
	}
}

func verifyLoggedAccessBenchmarkOutput(b *testing.B, logFile *os.File, tc loggedAccessBenchmarkCase) {
	b.Helper()
	if _, err := logFile.Seek(0, io.SeekStart); err != nil {
		b.Fatal(err)
	}

	want := fmt.Sprintf(`"GET %s HTTP/1.1" %d %d`, tc.target, tc.status, len(tc.responseBody))
	scanner := bufio.NewScanner(logFile)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, want) {
			b.Fatalf("access log line %q does not contain %q", line, want)
		}
		if tc.userAgent != "" && !strings.Contains(line, `"`+tc.userAgent+`"`) {
			b.Fatalf("access log line %q does not contain user agent %q", line, tc.userAgent)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		b.Fatal(err)
	}
	if count != b.N {
		b.Fatalf("wrote %d access lines for %d requests", count, b.N)
	}
}
