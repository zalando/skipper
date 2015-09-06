package innkeeper

import (
	"encoding/json"
	"github.com/zalando/skipper/skipper"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

func innkeeperHandler(data *[]*routeData) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, err := json.Marshal(*data); err == nil {
			w.Write(b)
		} else {
			w.WriteHeader(500)
		}
	})
}

func sortDoc(doc string) string {
	exprs := strings.Split(doc, ";")
	sort.Strings(exprs)
	return strings.Join(exprs, ";")
}

func checkDoc(out skipper.RawData, in []*routeData) bool {
	doc := make(map[int64]string)
	updateDoc(doc, in)
	return sortDoc(toDocument(doc).Get()) == sortDoc(out.Get())
}

func TestNothingToReceive(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	api := httptest.NewServer(http.NotFoundHandler())
	client := Make(api.URL, pollingTimeout)
	select {
	case <-client.Receive():
		t.Error("shoudn't have received anything")
	case <-time.After(2 * pollingTimeout):
		// test done
	}
}

func TestReceiveInitialDataImmediately(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := []*routeData{
		{1, false, `Path("/") -> "https://example.org"`},
		{2, true, `Path("/catalog") -> "https://example.org/catalog"`},
		{3, false, `Path("/catalog") -> "https://example.org/new-catalog"`}}
	api := httptest.NewServer(innkeeperHandler(&data))
	client := Make(api.URL, pollingTimeout)
	select {
	case doc := <-client.Receive():
		if !checkDoc(doc, []*routeData{
			{1, false, `Path("/") -> "https://example.org"`},
			{3, false, `Path("/catalog") -> "https://example.org/new-catalog"`}}) {

			t.Error("failed to receive the right data")
		}
	case <-time.After(pollingTimeout):
		t.Error("timeout")
	}
}

func TestReceiveNew(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := []*routeData{
		{1, false, `Path("/") -> "https://example.org"`},
		{2, true, `Path("/catalog") -> "https://example.org/catalog"`},
		{3, false, `Path("/catalog") -> "https://example.org/new-catalog"`}}
	api := httptest.NewServer(innkeeperHandler(&data))
	client := Make(api.URL, pollingTimeout)

	// receive initial
	<-client.Receive()

	// make a change
	data = append(data, &routeData{4, false, `Path("/pdp") -> "https://example.org/pdp"`})

	// wait for the change
	select {
	case doc := <-client.Receive():
		if !checkDoc(doc, []*routeData{
			{1, false, `Path("/") -> "https://example.org"`},
			{3, false, `Path("/catalog") -> "https://example.org/new-catalog"`},
			{4, false, `Path("/pdp") -> "https://example.org/pdp"`}}) {

			t.Error("failed to receive the right data")
		}
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}
}

func TestReceiveUpdate(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := []*routeData{
		{1, false, `Path("/") -> "https://example.org"`},
		{2, true, `Path("/catalog") -> "https://example.org/catalog"`},
		{3, false, `Path("/catalog") -> "https://example.org/new-catalog"`}}
	api := httptest.NewServer(innkeeperHandler(&data))
	client := Make(api.URL, pollingTimeout)

	// receive initial
	<-client.Receive()

	// make a change
	data[2].Route = `Path("/catalog") -> "https://example.org/extra-catalog"`

	// wait for the change
	select {
	case doc := <-client.Receive():
		if !checkDoc(doc, []*routeData{
			{1, false, `Path("/") -> "https://example.org"`},
			{3, false, `Path("/catalog") -> "https://example.org/extra-catalog"`}}) {

			t.Error("failed to receive the right data")
		}
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}
}

func TestReceiveDelete(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := []*routeData{
		{1, false, `Path("/") -> "https://example.org"`},
		{2, true, `Path("/catalog") -> "https://example.org/catalog"`},
		{3, false, `Path("/catalog") -> "https://example.org/new-catalog"`}}
	api := httptest.NewServer(innkeeperHandler(&data))
	client := Make(api.URL, pollingTimeout)

	// recieve initial
	<-client.Receive()

	// make a change
	data[2].Deleted = true

	// wait for the change
	select {
	case doc := <-client.Receive():
		if !checkDoc(doc, []*routeData{{1, false, `Path("/") -> "https://example.org"`}}) {
			t.Error("failed to receive the right data")
		}
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}
}

func TestNoChange(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := []*routeData{
		{1, false, `Path("/") -> "https://example.org"`},
		{2, true, `Path("/catalog") -> "https://example.org/catalog"`},
		{3, false, `Path("/catalog") -> "https://example.org/new-catalog"`}}
	api := httptest.NewServer(innkeeperHandler(&data))
	client := Make(api.URL, pollingTimeout)

	// recieve initial
	<-client.Receive()

	// check if receives anything
	select {
	case <-client.Receive():
		t.Error("shouldn't have received a change")
	case <-time.After(2 * pollingTimeout):
		// test done
	}
}
