package middleware

// Ported from Goji's middleware, source:
// https://github.com/zenazn/goji/tree/master/web/middleware

import (
	"bytes"
	"log"
	"runtime/debug"

	"github.com/valyala/fasthttp"

	"bitbucket.org/gle/chi"
	"golang.org/x/net/context"
)

// Recoverer is a middleware that recovers from panics, logs the panic (and a
// backtrace), and returns a HTTP 500 (Internal Server Error) status if
// possible.
//
// Recoverer prints a request ID if one is provided.
func Recoverer(next chi.Handler) chi.Handler {
	fn := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		defer func() {
			if err := recover(); err != nil {
				debug.PrintStack()
				fctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
			}
		}()

		next.ServeHTTPC(ctx, fctx)
	}

	return chi.HandlerFunc(fn)
}

func printPanic(buf *bytes.Buffer, reqID string, err interface{}) {
	cW(buf, bRed, "panic: %+v", err)
	log.Print(buf.String())
}
