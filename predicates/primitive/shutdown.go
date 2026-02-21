package primitive

import (
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"

	log "github.com/sirupsen/logrus"
)

type shutdown struct {
	inShutdown int32
}

// NewShutdown provides a predicate spec to create predicates
// that evaluate to true if Skipper is shutting down
func NewShutdown() routing.PredicateSpec {
	s, _ := newShutdown()
	return s
}

func newShutdown() (routing.PredicateSpec, chan os.Signal) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	s := &shutdown{}
	go func() {
		<-sigs
		log.Infof("Got shutdown signal for %s predicates", s.Name())
		atomic.StoreInt32(&s.inShutdown, 1)
	}()
	return s, sigs
}

func (*shutdown) Name() string { return predicates.ShutdownName }

// Create returns a Predicate that evaluates to true if Skipper is shutting down
func (s *shutdown) Create(args []any) (routing.Predicate, error) {
	if len(args) != 0 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	return s, nil
}

func (s *shutdown) Match(*http.Request) bool {
	return atomic.LoadInt32(&s.inShutdown) != 0
}
