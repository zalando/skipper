package diag

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestLogHeader(t *testing.T) {
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	req, err := http.NewRequest("GET", "https://example.org", nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &filtertest.Context{
		FRequest: req,
	}

	outputVerify := bytes.NewBuffer(nil)
	req.Body = nil
	if err := req.Write(outputVerify); err != nil {
		t.Fatal(err)
	}

	loggerVerify := bytes.NewBuffer(nil)
	log.SetOutput(loggerVerify)
	log.Println(outputVerify.String())

	loggerTest := bytes.NewBuffer(nil)
	log.SetOutput(loggerTest)

	req.Body = ioutil.NopCloser(bytes.NewBufferString("foo bar baz"))
	(logHeader{}).Request(ctx)
	if loggerTest.Len() == 0 || !bytes.Equal(loggerTest.Bytes(), loggerVerify.Bytes()) {
		t.Error("failed to log the request header")
		t.Log("expected:")
		t.Log(loggerVerify.String())
		t.Log("got:")
		t.Log(loggerTest.String())
	}
}
