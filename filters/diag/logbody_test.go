package diag

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestLogBody(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	t.Run("Request", func(t *testing.T) {
		beRoutes := eskip.MustParse(`r: * -> absorbSilent() -> repeatContent("a", 10) -> <shunt>`)
		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewAbsorbSilent())
		fr.Register(NewRepeat())
		be := proxytest.New(fr, beRoutes...)
		defer be.Close()

		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("request") -> "%s"`, be.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		content := "testrequest"
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(content)
		rsp, err := http.DefaultClient.Post(p.URL, "text/plain", buf)
		log.SetOutput(os.Stderr)
		if err != nil {
			t.Fatalf("Failed to POST: %v", err)
		}
		defer rsp.Body.Close()

		if got := logbuf.String(); !strings.Contains(got, content) {
			t.Fatalf("Failed to find %q log, got: %q", content, got)
		}
	})

	t.Run("Request is default", func(t *testing.T) {
		beRoutes := eskip.MustParse(`r: * -> absorbSilent() -> repeatContent("a", 10) -> <shunt>`)
		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewAbsorbSilent())
		fr.Register(NewRepeat())
		be := proxytest.New(fr, beRoutes...)
		defer be.Close()

		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody() -> "%s"`, be.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		content := "testrequestisdefault"
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(content)
		rsp, err := http.DefaultClient.Post(p.URL, "text/plain", buf)
		log.SetOutput(os.Stderr)
		if err != nil {
			t.Fatalf("Failed to POST: %v", err)
		}
		defer rsp.Body.Close()

		if got := logbuf.String(); !strings.Contains(got, content) {
			t.Fatalf("Failed to find %q log, got: %q", content, got)
		}
	})

	t.Run("Response", func(t *testing.T) {
		beRoutes := eskip.MustParse(`r: * -> repeatContent("a", 10) -> <shunt>`)
		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewRepeat())
		be := proxytest.New(fr, beRoutes...)
		defer be.Close()

		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("response") -> "%s"`, be.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		content := "testrequest"
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(content)
		rsp, err := http.DefaultClient.Post(p.URL, "text/plain", buf)
		if err != nil {
			t.Fatalf("Failed to do post request: %v", err)
		}

		defer rsp.Body.Close()
		io.Copy(io.Discard, rsp.Body)
		log.SetOutput(os.Stderr)

		got := logbuf.String()
		if strings.Contains(got, content) {
			t.Fatalf("Found request body %q in %q", content, got)
		}
		// repeatContent("a", 10)
		if !strings.Contains(got, "aaaaaaaaaa") {
			t.Fatalf("Failed to find rsp content %q log, got: %q", "aaaaaaaaaa", got)
		}
	})
	t.Run("Request-response", func(t *testing.T) {
		beRoutes := eskip.MustParse(`r: * -> repeatContent("a", 10) -> <shunt>`)
		fr := make(filters.Registry)
		fr.Register(NewLogBody())
		fr.Register(NewRepeat())
		be := proxytest.New(fr, beRoutes...)
		defer be.Close()

		routes := eskip.MustParse(fmt.Sprintf(`r: * -> logBody("request","response") -> "%s"`, be.URL))
		p := proxytest.New(fr, routes...)
		defer p.Close()

		requestContent := "testrequestresponse"
		logbuf := bytes.NewBuffer(nil)
		log.SetOutput(logbuf)
		buf := bytes.NewBufferString(requestContent)
		rsp, err := http.DefaultClient.Post(p.URL, "text/plain", buf)
		if err != nil {
			t.Fatalf("Failed to get respone: %v", err)
		}
		defer rsp.Body.Close()
		io.Copy(io.Discard, rsp.Body)
		log.SetOutput(os.Stderr)

		got := logbuf.String()
		if !strings.Contains(got, requestContent) {
			t.Fatalf("Failed to find req %q log, got: %q", requestContent, got)
		}
		// repeatContent("a", 10)
		if !strings.Contains(got, "aaaaaaaaaa") {
			t.Fatalf("Failed to find %q log, got: %q", "aaaaaaaaaa", got)
		}
	})
}
