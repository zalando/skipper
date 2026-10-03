package testdataclient

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

func TestNewDoc(t *testing.T) {
	t.Run("valid doc", func(t *testing.T) {
		client, err := NewDoc(`r1: Path("/r1") -> <shunt>;`)
		require.NoError(t, err)
		require.NotNil(t, client)

		routes, err := client.LoadAll()
		require.NoError(t, err)
		assert.Len(t, routes, 1)
		assert.Equal(t, "r1", routes[0].Id)
	})

	t.Run("invalid doc returns error", func(t *testing.T) {
		client, err := NewDoc(`invalid eskip syntax {{{`)
		assert.Error(t, err)
		assert.Nil(t, client)
	})
}

func TestLoadUpdateAndUpdate(t *testing.T) {
	t.Run("update and load update successfully", func(t *testing.T) {
		initial := []*eskip.Route{
			{Id: "r1", Path: "/r1"},
			{Id: "r2", Path: "/r2"},
		}
		client := New(initial)

		upsert := []*eskip.Route{
			{Id: "r3", Path: "/r3"},
		}
		deleted := []string{"r1"}

		go client.Update(upsert, deleted)

		u, d, err := client.LoadUpdate()
		require.NoError(t, err)
		assert.Equal(t, upsert, u)
		assert.Equal(t, deleted, d)

		// Verify internal route map reflection via LoadAll
		all, err := client.LoadAll()
		require.NoError(t, err)
		routeMap := make(map[string]*eskip.Route)
		for _, r := range all {
			routeMap[r.Id] = r
		}
		assert.NotContains(t, routeMap, "r1")
		assert.Contains(t, routeMap, "r2")
		assert.Contains(t, routeMap, "r3")
	})

	t.Run("load update fail next", func(t *testing.T) {
		client := New(nil)
		client.FailNext()

		go client.Update([]*eskip.Route{{Id: "r1"}}, nil)

		u, d, err := client.LoadUpdate()
		assert.Error(t, err)
		assert.Nil(t, u)
		assert.Nil(t, d)
	})
}

func TestUpdateDoc(t *testing.T) {
	t.Run("valid doc update", func(t *testing.T) {
		client := New(nil)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := client.UpdateDoc(`r1: Path("/foo") -> <shunt>;`, []string{"r0"})
			assert.NoError(t, err)
		}()

		u, d, err := client.LoadUpdate()
		wg.Wait()
		require.NoError(t, err)
		require.Len(t, u, 1)
		assert.Equal(t, "r1", u[0].Id)
		assert.Equal(t, []string{"r0"}, d)
	})

	t.Run("invalid doc returns error", func(t *testing.T) {
		client := New(nil)
		err := client.UpdateDoc(`invalid eskip syntax {{{`, nil)
		assert.Error(t, err)
	})
}

func TestWithLoadAllDelay(t *testing.T) {
	client := New(nil)
	delay := 30 * time.Millisecond
	client.WithLoadAllDelay(delay)

	start := time.Now()
	_, err := client.LoadAll()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), delay)
}

func TestClose(t *testing.T) {
	client := New(nil)

	done := make(chan struct{})
	go func() {
		u, d, err := client.LoadUpdate()
		assert.NoError(t, err)
		assert.Nil(t, u)
		assert.Nil(t, d)
		close(done)
	}()

	client.Close()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for LoadUpdate to return on Close")
	}
}
