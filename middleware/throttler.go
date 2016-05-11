package middleware

import (
	"time"

	"bitbucket.org/gle/chi"
	"github.com/valyala/fasthttp"
	"golang.org/x/net/context"
)

const (
	errCapacityExceeded = "Server capacity exceeded."
	errTimedOut         = "Timed out while waiting for a pending request to complete."
	errContextCanceled  = "Context was canceled."
)

var (
	defaultBacklogTimeout = time.Second * 60
)

// Throttle is a middleware that limits number of currently processed requests
// at a time.
func Throttle(limit int) func(chi.Handler) chi.Handler {
	return ThrottleBacklog(limit, 0, defaultBacklogTimeout)
}

// ThrottleBacklog is a middleware that limits number of currently processed
// requests at a time and provides a backlog for holding a finite number of
// pending requests.
func ThrottleBacklog(limit int, backlogLimit int, backlogTimeout time.Duration) func(chi.Handler) chi.Handler {
	if limit < 1 {
		panic("middleware.Throttle expects limit > 0")
	}

	if backlogLimit < 0 {
		panic("middleware.Throttle expects backlogLimit to be positive")
	}

	t := throttler{
		tokens:         make(chan token, limit),
		backlogTokens:  make(chan token, limit+backlogLimit),
		backlogTimeout: backlogTimeout,
	}

	// Filling tokens.
	for i := 0; i < limit+backlogLimit; i++ {
		if i < limit {
			t.tokens <- token{}
		}
		t.backlogTokens <- token{}
	}

	fn := func(h chi.Handler) chi.Handler {
		t.h = h
		return &t
	}

	return fn
}

// token represents a request that is being processed.
type token struct{}

// throttler limits number of currently processed requests at a time.
type throttler struct {
	h              chi.Handler
	tokens         chan token
	backlogTokens  chan token
	backlogTimeout time.Duration
}

// ServeHTTPC implements chi.Handler interface.
func (t *throttler) ServeHTTPC(ctx context.Context, fctx *fasthttp.RequestCtx) {
	select {
	case <-ctx.Done():
		fctx.Error(errContextCanceled, fasthttp.StatusServiceUnavailable)
		return
	case btok := <-t.backlogTokens:
		timer := time.NewTimer(t.backlogTimeout)

		defer func() {
			t.backlogTokens <- btok
		}()

		select {
		case <-timer.C:
			fctx.Error(errTimedOut, fasthttp.StatusServiceUnavailable)
			return
		case <-ctx.Done():
			fctx.Error(errContextCanceled, fasthttp.StatusServiceUnavailable)
			return
		case tok := <-t.tokens:
			defer func() {
				t.tokens <- tok
			}()
			// t.h.ServeHTTPC(ctx, w, r)
			t.h.ServeHTTPC(ctx, fctx)
		}
		return
	default:
		fctx.Error(errCapacityExceeded, fasthttp.StatusServiceUnavailable)
		return
	}
}
