package routing

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/requestmatch"
    "log"
    "net/http"
)

type DataClient interface {
    Receive() <-chan string
}

type Backend struct {
    Scheme string
    Host string
    Shunt bool
}

type Route struct {
    *Backend
	Filters []filters.Filter
}

type Routing struct {
    filterRegistry filters.Registry
    ignoreTrailingSlash bool
    getMatcher <-chan *requestmatch.Matcher
}

func feedMatchers(current *requestmatch.Matcher) (chan<- *requestmatch.Matcher, <-chan *requestmatch.Matcher) {
    // todo: measure impact of buffered channel here for out
    in := make(chan *requestmatch.Matcher)
    out := make(chan *requestmatch.Matcher)

    go func() {
        for {
            select {
            case current = <-in:
            case out <- current:
            }
        }
    }()

    return in, out
}

func (r *Routing) receiveUpdates(in <-chan string, out chan<- *requestmatch.Matcher) {
    go func() {
        for {
            data := <-in
            matcher, err := r.processData(data)
            if err != nil {
                // only logging errors here, and waiting for settings from external
                // sources to be fixed
                log.Println("error while processing route data", err)
                continue
            }

            out <- matcher
        }
    }()
}

func New(dc DataClient, fr filters.Registry, ignoreTrailingSlash bool) *Routing {
    r := &Routing{filterRegistry: fr, ignoreTrailingSlash: ignoreTrailingSlash}
    initialMatcher := r.createMatcher(nil)
    matchersIn, matchersOut := feedMatchers(initialMatcher)
    r.receiveUpdates(dc.Receive(), matchersIn)
    r.getMatcher = matchersOut
	return r
}

func (r *Routing) Route(req *http.Request) (*Route, map[string]string) {
    m := <-r.getMatcher
	value, params := m.Match(req)
    route, _ := value.(*Route)
	return route, params
}
