package backendtest

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestServerShouldCloseWhenAllRequestsAreFulfilled(t *testing.T) {
	expectedRequests := 4
	recorder := NewBackendRecorder(expectedRequests)
	for i := 0; i < expectedRequests; i++ {
		go func(counter int) {
			resp, err := http.Get(recorder.GetURL() + "/" + strconv.Itoa(counter))
			if err != nil {
				t.Error(err)
			}
			c, _ := ioutil.ReadAll(resp.Body)
			println(string(c))
		}(i)
	}
	select {
	case <-recorder.Done:
		if recorder.GetServedRequests() != expectedRequests {
			t.Errorf("number of requests served does not match with the expected. Expected %d but got %d", expectedRequests, recorder.GetServedRequests())
		}
	case <-time.After(time.Second * 1):
		t.Errorf(
			"only %d requests were served but %d were issued",
			recorder.GetServedRequests(),
			recorder.GetExpectedRequests(),
		)
	}
}
func TestServerShouldTimeoutMissingRequestsToResolve(t *testing.T) {
	recorder := NewBackendRecorder(4)
	for i := 0; i < 3; i++ {
		go func(counter int) {
			resp, err := http.Get(recorder.GetURL() + "/" + strconv.Itoa(counter))
			if err != nil {
				t.Error(err)
			}
			_, _ = ioutil.ReadAll(resp.Body)
		}(i)
	}
	select {
	case <-recorder.Done:
		t.Error("expected to fail with pending requests")
	case <-time.After(time.Second * 1):

	}
}
