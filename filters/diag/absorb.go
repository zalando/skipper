package diag

import (
	"io"
	"net/http"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/logging"
)

// AbsorbName contains the name of the absorb filter.
const AbsorbName = "absorb"

// AbsorbSilentName contains the name of the absorbSilent filter.
const AbsorbSilentName = "absorbSilent"

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

func (a absorb) Name() string                                            { return AbsorbName }
func (a absorb) CreateFilter(args []interface{}) (filters.Filter, error) { return a, nil }
func (a absorb) Response(filters.FilterContext)                          {}

func (a absorb) Request(ctx filters.FilterContext) {
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

	if !a.silent {
		a.logger.Infof("received request to be absorbed: %s", id)
	}

	var count = 0
	buf := make([]byte, 1<<12)
	for {
		n, err := req.Body.Read(buf)
		count += n
		if !a.silent {
			a.logger.Infof("request %s, consumed bytes: %d", id, count)
		}

		if err != nil {
			if err != io.EOF {
				a.logger.Infof("request %s, error while consuming request: %v", id, err)
			}

			break
		}
	}

	if !a.silent {
		a.logger.Infof("request %s, consumed bytes: %d", id, count)
		a.logger.Infof("request finished: %s", id)
	}

	ctx.Serve(&http.Response{StatusCode: http.StatusOK})
}
