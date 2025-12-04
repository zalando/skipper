package xxray

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Propagator is an AWS X-Ray trace propagator that extends the standard [xray.Propagator].
// Standard propagator requires both Root and Parent keys to be present in the X-Amzn-Trace-Id header
// to successfully extract span context.
// AWS [ALB request tracing] creates X-Amzn-Trace-Id header with only Root field - this propagator
// can re-use it to obtain trace ID value.
//
// [ALB request tracing]: https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-request-tracing.html
type Propagator struct {
	xray.Propagator
	idGenerator *xray.IDGenerator
}

func NewPropagator() *Propagator {
	return &Propagator{idGenerator: xray.NewIDGenerator()}
}

func (p *Propagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	newCtx := p.Propagator.Extract(ctx, carrier)
	// If failed to extract span context, try to re-use trace id
	if newCtx == ctx {
		if header := carrier.Get(traceHeaderKey); header != "" {
			tsc, err := extract(header)
			if err == nil && tsc.TraceID().IsValid() {
				// Re-use only trace id
				return trace.ContextWithRemoteSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
					TraceID: tsc.TraceID(),
					SpanID:  p.idGenerator.NewSpanID(ctx, tsc.TraceID()),
				}))
			}
		}
	}
	return newCtx
}

// The rest is copied from https://github.com/open-telemetry/opentelemetry-go-contrib/blob/80c9316336ebb4f4c67d8e1011a3add889213fb7/propagators/aws/xray/propagator.go
const (
	traceHeaderKey       = "X-Amzn-Trace-Id"
	traceHeaderDelimiter = ";"
	kvDelimiter          = "="
	traceIDKey           = "Root"
	sampleFlagKey        = "Sampled"
	parentIDKey          = "Parent"
	traceIDVersion       = "1"
	traceIDDelimiter     = "-"
	isSampled            = "1"
	notSampled           = "0"

	traceFlagNone           = 0x0
	traceFlagSampled        = 0x1 << 0
	traceIDLength           = 35
	traceIDDelimitterIndex1 = 1
	traceIDDelimitterIndex2 = 10
	traceIDFirstPartLength  = 8
	sampledFlagLength       = 1
)

var (
	empty                    = trace.SpanContext{}
	errInvalidTraceHeader    = errors.New("invalid X-Amzn-Trace-Id header value, should contain 3 different part separated by ;")
	errMalformedTraceID      = errors.New("cannot decode trace ID from header")
	errLengthTraceIDHeader   = errors.New("incorrect length of X-Ray trace ID found, 35 character length expected")
	errInvalidTraceIDVersion = errors.New("invalid X-Ray trace ID header found, does not have valid trace ID version")
	errInvalidSpanIDLength   = errors.New("invalid span ID length, must be 16")
)

// extract extracts Span Context from context.
func extract(headerVal string) (trace.SpanContext, error) {
	var (
		scc            = trace.SpanContextConfig{}
		err            error
		delimiterIndex int
		part           string
	)
	pos := 0
	for pos < len(headerVal) {
		delimiterIndex = indexOf(headerVal, traceHeaderDelimiter, pos)
		if delimiterIndex >= 0 {
			part = headerVal[pos:delimiterIndex]
			pos = delimiterIndex + 1
		} else {
			// last part
			part = strings.TrimSpace(headerVal[pos:])
			pos = len(headerVal)
		}
		equalsIndex := strings.Index(part, kvDelimiter)
		if equalsIndex < 0 {
			return empty, errInvalidTraceHeader
		}
		value := part[equalsIndex+1:]
		switch {
		case strings.HasPrefix(part, traceIDKey):
			scc.TraceID, err = parseTraceID(value)
			if err != nil {
				return empty, err
			}
		case strings.HasPrefix(part, parentIDKey):
			// extract parentId
			scc.SpanID, err = trace.SpanIDFromHex(value)
			if err != nil {
				return empty, errInvalidSpanIDLength
			}
		case strings.HasPrefix(part, sampleFlagKey):
			// extract traceflag
			scc.TraceFlags = parseTraceFlag(value)
		}
	}
	return trace.NewSpanContext(scc), nil
}

// indexOf returns position of the first occurrence of a substr in str starting at pos index.
func indexOf(str, substr string, pos int) int {
	index := strings.Index(str[pos:], substr)
	if index > -1 {
		index += pos
	}
	return index
}

// parseTraceID returns trace ID if  valid else return invalid trace ID.
func parseTraceID(xrayTraceID string) (trace.TraceID, error) {
	if len(xrayTraceID) != traceIDLength {
		return empty.TraceID(), errLengthTraceIDHeader
	}
	if !strings.HasPrefix(xrayTraceID, traceIDVersion) {
		return empty.TraceID(), errInvalidTraceIDVersion
	}

	if xrayTraceID[traceIDDelimitterIndex1:traceIDDelimitterIndex1+1] != traceIDDelimiter ||
		xrayTraceID[traceIDDelimitterIndex2:traceIDDelimitterIndex2+1] != traceIDDelimiter {
		return empty.TraceID(), errMalformedTraceID
	}

	epochPart := xrayTraceID[traceIDDelimitterIndex1+1 : traceIDDelimitterIndex2]
	uniquePart := xrayTraceID[traceIDDelimitterIndex2+1 : traceIDLength]

	result := epochPart + uniquePart
	return trace.TraceIDFromHex(result)
}

// parseTraceFlag returns a parsed trace flag.
func parseTraceFlag(xraySampledFlag string) trace.TraceFlags {
	// Use a direct comparison here (#7262).
	if xraySampledFlag == isSampled {
		return trace.FlagsSampled
	}
	return trace.FlagsSampled.WithSampled(false)
}
