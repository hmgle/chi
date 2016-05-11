package middleware

import (
	"time"

	"github.com/valyala/fasthttp"

	"bitbucket.org/gle/chi"
	"golang.org/x/net/context"
)

// Timeout is a middleware that cancels ctx after a given timeout.
func Timeout(timeout time.Duration) func(next chi.Handler) chi.Handler {
	return func(next chi.Handler) chi.Handler {
		fn := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer func() {
				cancel()
				if ctx.Err() == context.DeadlineExceeded {
					fctx.SetStatusCode(fasthttp.StatusGatewayTimeout)
				}
			}()

			next.ServeHTTPC(ctx, fctx)
		}
		return chi.HandlerFunc(fn)
	}
}
