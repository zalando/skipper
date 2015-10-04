package testdataclient

import (
    "testing"
    "time"
)

func TestInitEmpty(t *testing.T) {
    dc := New("")
    rs, _ := dc.Receive()
    if len(rs) != 0 {
        t.Error("init of empty data client failed")
    }
}

func TestFeedAfterInitEmpty(t *testing.T) {
    dc := New("")
    _, uc := dc.Receive()
    dc.Feed(`Any() -> "http://www.example.org"`, nil)
    u := <-uc
    if len(u.UpsertedRoutes) != 1 {
        t.Error("failed to feed update")
    }
}

func TestInitWithDoc(t *testing.T) {
    dc := New(`Any() -> "http://www.example.org"`)
    rs, _ := dc.Receive()
    if len(rs) != 1 {
        t.Error("failed to init data client")
    }
}

func TestDoNotSendUpdateOnInitialData(t *testing.T) {
    dc := New(`Any() -> "http://www.example.org"`)
    _, u := dc.Receive()
    select {
    case <-u:
        t.Error("should not have received an update")
    case <-time.After(3 * time.Millisecond):
    }
}
