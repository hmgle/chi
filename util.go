package chi

import (
	"fmt"
	"strings"

	"github.com/valyala/fasthttp"

	"golang.org/x/net/context"
)

// Build a chained chi.Handler from a list of middlewares
func chain(middlewares []interface{}, handlers ...interface{}) Handler {
	// join a middleware stack with inline middlewares
	mws := append(middlewares, handlers[:len(handlers)-1]...)

	// request handler
	handler := handlers[len(handlers)-1]

	// Assert the types in the middleware chain
	for _, mw := range mws {
		assertMiddleware(mw)
	}

	// Set the request handler to a context handler type
	var cxh Handler
	switch t := handler.(type) {
	default:
		panic(fmt.Sprintf("chi: unsupported handler signature: %T", t))
	case Handler:
		cxh = t
	case func(context.Context, *fasthttp.RequestCtx):
		cxh = HandlerFunc(t)
	case func(*fasthttp.RequestCtx):
		cxh = HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			t(fctx)
		})
	}

	// Return ahead of time if there aren't any middlewares for the chain
	if len(mws) == 0 {
		return cxh
	}

	// Wrap the end handler with the middleware chain
	h := mwrap(mws[len(mws)-1])(cxh)
	for i := len(mws) - 2; i >= 0; i-- {
		f := mwrap(mws[i])
		h = f(h)
	}

	return h
}

// Wrap http.Handler middleware to chi.Handler middlewares
func mwrap(middleware interface{}) func(Handler) Handler {
	switch mw := middleware.(type) {
	default:
		panic(fmt.Sprintf("chi: unsupported handler signature: %T", mw))

	case func(Handler) Handler:
		return mw
	}
}

// Runtime type checking of the middleware signature
func assertMiddleware(middleware interface{}) interface{} {
	switch t := middleware.(type) {
	default:
		panic(fmt.Sprintf("chi: unsupported middleware signature: %T", t))
	case func(Handler) Handler:
	}
	return middleware
}

// Respond with just the allowed methods, as required by RFC2616 for
// 405 Method not allowed.
func methodNotAllowedHandler(ctx context.Context, fctx *fasthttp.RequestCtx) {
	methods := make([]string, len(methodMap))
	i := 0
	for m := range methodMap {
		methods[i] = m // still faster than append to array with capacity
		i++
	}

	fctx.Response.Header.Add("Allow", strings.Join(methods, ","))
	fctx.SetStatusCode(405)
	fctx.Write([]byte("Method Not Allowed"))
}
