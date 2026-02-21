package flowid

import (
	"bytes"
	"fmt"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"log"
	"net/http"
	"strings"
	"testing"
)

const (
	testFlowId    = "FLOW-ID-FOR-TESTING"
	invalidFlowId = "[<>] (o) [<>]"
)

var (
	testFlowIdSpec           = New()
	filterConfigWithReuse    = []any{ReuseParameterValue}
	filterConfigWithoutReuse = []any{"dummy"}
)

func TestNewFlowIdGeneration(t *testing.T) {
	f, _ := testFlowIdSpec.CreateFilter(filterConfigWithReuse)
	fc := buildfilterContext()
	f.Request(fc)

	flowId := fc.Request().Header.Get(HeaderName)
	if flowId == "" {
		t.Error("flowId not generated")
	}
}

func TestFlowIdReuseExisting(t *testing.T) {
	f, _ := testFlowIdSpec.CreateFilter(filterConfigWithReuse)
	fc := buildfilterContext(HeaderName, testFlowId)
	f.Request(fc)

	flowId := fc.Request().Header.Get(HeaderName)
	if flowId != testFlowId {
		t.Errorf("Got wrong flow id. Expected '%s' got '%s'", testFlowId, flowId)
	}
}

func TestFlowIdIgnoreReuseExisting(t *testing.T) {
	f, _ := testFlowIdSpec.CreateFilter(filterConfigWithoutReuse)
	fc := buildfilterContext(HeaderName, testFlowId)
	f.Request(fc)

	flowId := fc.Request().Header.Get(HeaderName)
	if flowId == testFlowId {
		t.Errorf("Got wrong flow id. Expected a newly generated flowid but got the test flow id '%s'", flowId)
	}
}

func TestFlowIdRejectInvalidReusedFlowId(t *testing.T) {
	f, _ := testFlowIdSpec.CreateFilter(filterConfigWithReuse)
	fc := buildfilterContext(HeaderName, invalidFlowId)
	f.Request(fc)

	flowId := fc.Request().Header.Get(HeaderName)
	if flowId == invalidFlowId {
		t.Errorf("Got wrong flow id. Expected a newly generated flowid but got the test flow id '%s'", flowId)
	}
}

func TestFlowIdWithInvalidParameters(t *testing.T) {
	fc := []any{true}
	_, err := testFlowIdSpec.CreateFilter(fc)
	if err != filters.ErrInvalidFilterParameters {
		t.Errorf("expected an invalid parameters error, got %v", err)
	}

	fc = []any{1}
	_, err = testFlowIdSpec.CreateFilter(fc)
	if err != filters.ErrInvalidFilterParameters {
		t.Errorf("expected an invalid parameters error, got %v", err)
	}
}

func TestDeprecationNotice(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	fc := []any{"", true}
	_, err := testFlowIdSpec.CreateFilter(fc)
	if err == filters.ErrInvalidFilterParameters {
		t.Error("unexpected error creating a filter")
	}

	logOutput := buf.String()
	if logOutput == "" {
		t.Error("no warning output produced")
	}

	if !(strings.Contains(logOutput, "warning") || strings.Contains(logOutput, "deprecated")) {
		t.Error("missing deprecation keywords from the output produced")
	}
}

func TestFlowIdWithCustomGenerators(t *testing.T) {
	for _, test := range []struct {
		generatorId string
	}{
		{""},
		{"builtin"},
		{"ulid"},
	} {
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			fc := []any{ReuseParameterValue, test.generatorId}
			f, _ := testFlowIdSpec.CreateFilter(fc)
			fctx := buildfilterContext()
			f.Request(fctx)

			flowId := fctx.Request().Header.Get(HeaderName)

			l := len(flowId)
			if l == 0 {
				t.Errorf("wrong flowId len. expected > 0 got %d / %q", l, flowId)
			}

		})
	}
}

func buildfilterContext(headers ...string) filters.FilterContext {
	r, _ := http.NewRequest("GET", "http://example.org", nil)
	for i := 0; i < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	return &filtertest.Context{FRequest: r}
}
