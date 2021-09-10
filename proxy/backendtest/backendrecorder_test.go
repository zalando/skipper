package backendtest

import (
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestServerShouldCloseWhenAllRequestsAreFulfilled(t *testing.T) {
	expectedRequests := 4
	recorder := NewBackendRecorder(10 * time.Millisecond)
	for i := 0; i < expectedRequests; i++ {
		go func(counter int) {
			resp, err := http.Get(recorder.GetURL() + "/" + strconv.Itoa(counter))
			if err != nil {
				t.Error(err)
			}
			_, _ = io.ReadAll(resp.Body)
		}(i)
	}
	<-recorder.Done
	servedRequests := len(recorder.GetRequests())
	if servedRequests != expectedRequests {
		t.Errorf("number of requests served does not match with the expected. Expected %d but got %d", expectedRequests, servedRequests)
	}
}
