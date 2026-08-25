package diag

import (
	"io"
	"net/http"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/logging"
)

// AbsorbName contains the name of the absorb filter.
// Deprecated, use filters.AbsorbName instead
const AbsorbName = filters.AbsorbName

// AbsorbSilentName contains the name of the absorbSilent filter.
// Deprecated, use filters.AbsorbSilentName instead
const AbsorbSilentName = filters.AbsorbSilentName

const loggingInterval = time.Second

type absorb struct {
	logger logging.Logger
	id     flowid.Generator
	silent bool
}

func withLogger(silent bool, l logging.Logger) filters.Spec {
	if l == nil {
		l = &logging.DefaultLog{}
	}

	id, err := flowid.NewStandardGenerator(flowid.MinLength)
	if err != nil {
		l.Errorf("failed to create ID generator: %v", err)
	}

	return &absorb{
		logger: l,
		id:     id,
		silent: silent,
	}
}

// NewAbsorb initializes a filter spec for the absorb filter.
//
// The absorb filter reads and discards the payload of the incoming requests.
// It logs with INFO level and a unique ID per request:
// - the event of receiving the request
// - partial and final events for consuming request payload and total consumed byte count
// - the finishing event of the request
// - any read errors other than EOF
func NewAbsorb() filters.Spec {
	return withLogger(false, nil)
}

// NewAbsorbSilent initializes a filter spec for the absorbSilent filter,
// similar to the absorb filter, but without verbose logging of the absorbed
// payload.
//
// The absorbSilent filter reads and discards the payload of the incoming requests. It only
// logs read errors other than EOF.
func NewAbsorbSilent() filters.Spec {
	return withLogger(true, nil)
}

func (a *absorb) Name() string {
	if a.silent {
		return filters.AbsorbSilentName
	}
	return filters.AbsorbName
}

func (a *absorb) CreateFilter(args []any) (filters.Filter, error) { return a, nil }
func (a *absorb) Response(filters.FilterContext)                  {}

func (a *absorb) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	id := req.Header.Get(flowid.HeaderName)
	if id == "" {
		if a.id == nil {
			id = "-"
		} else {
			var err error
			if id, err = a.id.Generate(); err != nil {
				a.logger.Error(err)
			}
		}
	}

	sink := io.Discard
	if !a.silent {
		a.logger.Infof("received request to be absorbed: %s", id)
		sink = &loggingSink{id: id, logger: a.logger, next: time.Now().Add(loggingInterval)}
	}

	count, err := io.Copy(sink, req.Body)

	if !a.silent {
		if err != nil {
			a.logger.Infof("request %s, error while consuming request: %v", id, err)
		}
		a.logger.Infof("request %s, consumed bytes: %d", id, count)
		a.logger.Infof("request finished: %s", id)
	}

	ctx.Serve(&http.Response{StatusCode: http.StatusOK})
}

type loggingSink struct {
	id     string
	logger logging.Logger
	next   time.Time
	count  int64
}

func (s *loggingSink) Write(p []byte) (n int, err error) {
	n, err = len(p), nil
	s.count += int64(n)
	if time.Now().After(s.next) {
		s.logger.Infof("request %s, consumed bytes: %d", s.id, s.count)
		s.next = s.next.Add(loggingInterval)
	}
	return
}
