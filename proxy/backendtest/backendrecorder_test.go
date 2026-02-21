package backendtest

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestServerShouldCloseWhenAllRequestsAreFulfilled(t *testing.T) {
	expectedRequests := 4
	recorder := NewBackendRecorder(10 * time.Millisecond)

	var g errgroup.Group
	for range expectedRequests {
		g.Go(func() error {
			resp, err := http.Get(recorder.GetURL())
			if err != nil {
				return err
			}
			if _, err := io.Copy(io.Discard, resp.Body); err != nil {
				return err
			}
			return resp.Body.Close()
		})
	}
	require.NoError(t, g.Wait())
	recorder.Done()

	assert.Equal(t, expectedRequests, len(recorder.GetRequests()))
}
