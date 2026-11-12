package proxy

import (
	stdlibcontext "context"
	"net/http"
	"net/url"
	"sync"
	"time"

	ot "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/tracing"
)

type backendAttemptExecution struct {
	attempt   *backendAttempt
	cancel    stdlibcontext.CancelFunc
	startedAt time.Time
	done      chan struct{}

	mu        sync.Mutex
	completed bool
	discarded bool
}

type hedgedBackendResult struct {
	attempt         *backendAttempt
	err             *proxyError
	primaryError    *proxyError
	secondStarted   bool
	winnerCancel    stdlibcontext.CancelFunc
	headerLatency   time.Duration
	requestElapsed  time.Duration
	responseElapsed time.Duration
}

func (e *backendAttemptExecution) completedResult() (*http.Response, *proxyError, time.Time, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.discarded {
		return nil, nil, time.Time{}, false
	}
	if e.attempt.headersReturned.Load() {
		return e.attempt.response, e.attempt.err, e.attempt.context.backendHeaderTime, true
	}
	if !e.completed {
		return nil, nil, time.Time{}, false
	}
	return e.attempt.response, e.attempt.err, e.attempt.context.backendHeaderTime, true
}

func (e *backendAttemptExecution) discard() {
	e.mu.Lock()
	if e.discarded {
		e.mu.Unlock()
		return
	}
	e.discarded = true
	var response *http.Response
	var span ot.Span
	if e.completed {
		response = e.attempt.response
		span = e.attempt.context.proxySpan
		e.attempt.response = nil
		e.attempt.context.proxySpan = nil
	}
	e.mu.Unlock()
	e.cancel()
	cleanupBackendAttempt(response, span)
}

func cleanupBackendAttempt(response *http.Response, span ot.Span) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if span != nil {
		span.Finish()
	}
}

func (p *Proxy) startBackendAttempt(attempt *backendAttempt, requestContext stdlibcontext.Context, results chan<- *backendAttemptExecution) *backendAttemptExecution {
	attemptContext, cancel := stdlibcontext.WithCancel(requestContext)
	execution := &backendAttemptExecution{
		attempt:   attempt,
		cancel:    cancel,
		startedAt: time.Now(),
		done:      make(chan struct{}),
	}
	go func() {
		response, err := p.makeBackendRequest(&attempt.context, attemptContext)
		execution.mu.Lock()
		var discardResponse *http.Response
		var discardSpan ot.Span
		if execution.discarded {
			discardResponse = response
			discardSpan = attempt.context.proxySpan
			attempt.response = nil
			attempt.context.proxySpan = nil
		} else {
			if !attempt.headersReturned.Load() {
				attempt.response = response
				attempt.err = err
			}
		}
		execution.completed = true
		close(execution.done)
		execution.mu.Unlock()
		cleanupBackendAttempt(discardResponse, discardSpan)
		results <- execution
	}()
	return execution
}

func hedgeContextError(ctx stdlibcontext.Context) *proxyError {
	if ctx.Err() == stdlibcontext.Canceled {
		return &proxyError{err: stdlibcontext.Canceled, code: 499}
	}
	return &proxyError{err: stdlibcontext.DeadlineExceeded, code: http.StatusGatewayTimeout}
}

func (p *Proxy) makeHedgedBackendRequest(ctx *context, requestContext stdlibcontext.Context, state filters.HedgeState, delay time.Duration) *hedgedBackendResult {
	primary := &backendAttempt{}
	primaryContext := ctx.newBackendAttemptContext(primary)
	primaryEndpoint := p.selectEndpoint(primaryContext)
	primaryContext.retryEndpoint = primaryEndpoint

	secondary := &backendAttempt{}
	secondaryContext := ctx.newBackendAttemptContext(secondary)
	if secondaryContext.request != nil && secondaryContext.request.URL != nil {
		secondaryContext.request.URL.Scheme = primaryEndpoint.Scheme
		secondaryContext.request.URL.Host = primaryEndpoint.Host
	}

	results := make(chan *backendAttemptExecution, 2)
	primaryExecution := p.startBackendAttempt(primary, requestContext, results)
	result := &hedgedBackendResult{}
	var secondaryExecution *backendAttemptExecution
	var primaryResult, secondaryResult *backendAttemptExecution
	var timer *time.Timer
	var timerC <-chan time.Time
	if delay < 0 {
		delay = 0
	}
	timer = time.NewTimer(delay)
	timerC = timer.C
	defer timer.Stop()

	startSecondary := func(requirePrimaryPending bool) bool {
		if secondaryExecution != nil || requestContext.Err() != nil {
			return false
		}
		reservation, ok := p.reserveRetryEndpoint(secondaryContext, &url.URL{Scheme: primaryEndpoint.Scheme, Host: primaryEndpoint.Host}, requestContext, state)
		if !ok {
			return false
		}
		if requestContext.Err() != nil || requirePrimaryPending && (primary.headersReturned.Load() || primaryExecution.completedResultState()) {
			reservation.release()
			return false
		}
		secondaryContext.retryEndpoint = reservation.endpoint
		if requestContext.Err() != nil {
			reservation.release()
			return false
		}
		secondaryExecution = p.startBackendAttempt(secondary, requestContext, results)
		reservation.commit()
		result.secondStarted = true
		return true
	}

	for {
		select {
		case execution := <-results:
			result.requestElapsed += execution.attempt.context.proxyRequestElapsed
			result.responseElapsed += execution.attempt.context.proxyResponseElapsed
			if execution == primaryExecution {
				primaryResult = execution
				if execution.attempt.err != nil {
					result.primaryError = execution.attempt.err
					if secondaryExecution != nil {
						p.markBackendRetry(ctx, execution.attempt)
					}
				}
				if execution.attempt.response != nil && execution.attempt.err == nil {
					winner := earlierBackendResponse(execution, secondaryExecution)
					return finishHedgedBackendRequest(result, winner, primaryExecution, secondaryExecution)
				}
				if secondaryExecution == nil && retryable(ctx, execution.attempt.err) && startSecondary(false) {
					p.markBackendRetry(ctx, execution.attempt)
					timerC = nil
					continue
				}
				if secondaryExecution == nil {
					return finishHedgedBackendRequest(result, execution, primaryExecution, secondaryExecution)
				}
				if secondaryResult != nil {
					return finishHedgedBackendRequest(result, secondaryResult, primaryExecution, secondaryExecution)
				}
			} else {
				secondaryResult = execution
				if execution.attempt.response != nil && execution.attempt.err == nil {
					winner := earlierBackendResponse(execution, primaryExecution)
					return finishHedgedBackendRequest(result, winner, primaryExecution, secondaryExecution)
				}
				if primaryResult != nil {
					return finishHedgedBackendRequest(result, execution, primaryExecution, secondaryExecution)
				}
			}
		case <-timerC:
			timerC = nil
			if requestContext.Err() != nil {
				if winner := completedResponseOnDeadline(primaryExecution, secondaryExecution); winner != nil {
					return finishHedgedBackendRequest(result, winner, primaryExecution, secondaryExecution)
				}
				primaryExecution.discard()
				if secondaryExecution != nil {
					secondaryExecution.discard()
				}
				result.err = hedgeContextError(requestContext)
				return result
			}
			if !primary.headersReturned.Load() && !primaryExecution.completedResultState() {
				startSecondary(true)
			}
		case <-requestContext.Done():
			if winner := completedResponseOnDeadline(primaryExecution, secondaryExecution); winner != nil {
				return finishHedgedBackendRequest(result, winner, primaryExecution, secondaryExecution)
			}
			primaryExecution.discard()
			if secondaryExecution != nil {
				secondaryExecution.discard()
			}
			result.err = hedgeContextError(requestContext)
			return result
		}
	}
}

func completedResponseOnDeadline(primary, secondary *backendAttemptExecution) *backendAttemptExecution {
	if primary.attempt.headersReturned.Load() {
		<-primary.done
	}
	if secondary != nil && secondary.attempt.headersReturned.Load() {
		<-secondary.done
	}
	var winner *backendAttemptExecution
	if response, err, _, completed := primary.completedResult(); completed && response != nil && err == nil {
		winner = primary
	}
	if secondary != nil {
		if response, err, _, completed := secondary.completedResult(); completed && response != nil && err == nil {
			if winner == nil {
				winner = secondary
			} else {
				winner = earlierBackendResponse(winner, secondary)
			}
		}
	}
	return winner
}

func earlierBackendResponse(candidate, other *backendAttemptExecution) *backendAttemptExecution {
	if other == nil {
		return candidate
	}
	response, err, otherHeaderTime, completed := other.completedResult()
	if completed && response != nil && err == nil && otherHeaderTime.Before(candidate.attempt.context.backendHeaderTime) {
		return other
	}
	return candidate
}

func finishHedgedBackendRequest(result *hedgedBackendResult, winner, primary, secondary *backendAttemptExecution) *hedgedBackendResult {
	<-winner.done
	result.attempt = winner.attempt
	result.err = winner.attempt.err
	result.winnerCancel = winner.cancel
	if winner.attempt.response != nil && winner.attempt.err == nil {
		result.headerLatency = winner.attempt.context.backendHeaderTime.Sub(primary.startedAt)
	}
	if winner != primary {
		primary.discard()
	}
	if secondary != nil && winner != secondary {
		secondary.discard()
	}
	return result
}

func (p *Proxy) markBackendRetry(ctx *context, attempt *backendAttempt) {
	if attempt.context.proxySpan != nil {
		p.tracing.setTag(attempt.context.proxySpan, "retry", ctx.route.Id)
	} else {
		tracing.LogKV("retry", ctx.route.Id, ctx.Request().Context())
	}
}

func (e *backendAttemptExecution) completedResultState() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.completed
}
