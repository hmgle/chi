package main

import (
	"fmt"

	"github.com/pressly/chi"
	"github.com/pressly/chi/middleware"
	"github.com/valyala/fasthttp"

	"golang.org/x/net/context"
)

func main() {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	// r.Use(middleware.RealIP)
	// r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(func(h chi.Handler) chi.Handler {
		return chi.HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			ctx = context.WithValue(ctx, "example", true)
			h.ServeHTTPC(ctx, fctx)
		})
	})

	r.Get("/", apiIndex)

	r.Mount("/accounts", accountsRouter())

	fasthttp.ListenAndServe(":3333", r.ServeHTTP)
}

func accountsRouter() chi.Router { // or http.Handler
	r := chi.NewRouter()

	r.Use(sup1)

	r.Get("/", listAccounts)
	r.Post("/", createAccount)
	r.Get("/hi", hiAccounts)

	r.Group(func(r chi.Router) {
		r.Use(sup2)

		r.Get("/hi2", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			v := ctx.Value("sup2").(string)
			fctx.Write([]byte(fmt.Sprintf("hi2 - '%s'", v)))
		})
		r.Get("/ahh", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			v := ctx.Value("sup2").(string)
			fctx.Write([]byte(fmt.Sprintf("ahh - '%s'", v)))
		})
		r.Get("/fail", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			panic("no..")
		})
	})

	r.Route("/:accountID", func(r chi.Router) {
		r.Use(accountCtx)
		r.Get("/", getAccount)
		r.Put("/", updateAccount)
		r.Get("/*", other)
	})

	return r
}

func sup1(next chi.Handler) chi.Handler {
	hfn := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		ctx = context.WithValue(ctx, "sup1", "sup1")
		next.ServeHTTPC(ctx, fctx)
	}
	return chi.HandlerFunc(hfn)
}

func sup2(next chi.Handler) chi.Handler {
	hfn := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		ctx = context.WithValue(ctx, "sup2", "sup2")
		next.ServeHTTPC(ctx, fctx)
	}
	return chi.HandlerFunc(hfn)
}

func accountCtx(h chi.Handler) chi.Handler {
	handler := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		ctx = context.WithValue(ctx, "account", "account 123")
		h.ServeHTTPC(ctx, fctx)
	}
	return chi.HandlerFunc(handler)
}

func apiIndex(ctx context.Context, fctx *fasthttp.RequestCtx) {
	fctx.Write([]byte("root"))
}

func listAccounts(ctx context.Context, fctx *fasthttp.RequestCtx) {
	fctx.Write([]byte("list accounts"))
}

func hiAccounts(ctx context.Context, fctx *fasthttp.RequestCtx) {
	sup1 := ctx.Value("sup1").(string)
	fctx.Write([]byte(fmt.Sprintf("hi accounts %v", sup1)))
}

func createAccount(fctx *fasthttp.RequestCtx) {
	fctx.Write([]byte("create account"))
}

func getAccount(ctx context.Context, fctx *fasthttp.RequestCtx) {
	accountID := chi.URLParam(ctx, "accountID")
	account := ctx.Value("account").(string)
	fctx.Write([]byte(fmt.Sprintf("get account id:%s details:%s", accountID, account)))
}

func updateAccount(ctx context.Context, fctx *fasthttp.RequestCtx) {
	account := ctx.Value("account").(string)
	fctx.Write([]byte(fmt.Sprintf("update account:%s", account)))
}

func other(fctx *fasthttp.RequestCtx) {
	fctx.Write([]byte("catch all.."))
}
