package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/pressly/chi"
	"github.com/pressly/chi/middleware"
	"github.com/pressly/chi/render"
	"github.com/valyala/fasthttp"

	"golang.org/x/net/context"
)

func main() {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	// r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", func(fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("..."))
	})

	r.Get("/ping", func(fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("pong"))
	})

	r.Get("/panic", func(fctx *fasthttp.RequestCtx) {
		panic("test")
	})

	// Slow handlers/operations.
	r.Group(func(r chi.Router) {
		// Stop processing when client disconnects.
		// TODO
		// r.Use(middleware.CloseNotify)

		// Stop processing after 2.5 seconds.
		r.Use(middleware.Timeout(2500 * time.Millisecond))

		r.Get("/slow", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			rand.Seed(time.Now().Unix())

			// Processing will take 1-5 seconds.
			processTime := time.Duration(rand.Intn(4)+1) * time.Second

			select {
			case <-ctx.Done():
				return

			case <-time.After(processTime):
				// The above channel simulates some hard work.
			}

			fctx.Write([]byte(fmt.Sprintf("Processed in %v seconds\n", processTime)))
		})
	})

	// Throttle very expensive handlers/operations.
	r.Group(func(r chi.Router) {
		// Stop processing after 30 seconds.
		r.Use(middleware.Timeout(30 * time.Second))

		// Only one request will be processed at a time.
		r.Use(middleware.Throttle(1))

		r.Get("/throttled", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			select {
			case <-ctx.Done():
				switch ctx.Err() {
				case context.DeadlineExceeded:
					fctx.SetStatusCode(504)
					fctx.Write([]byte("Processing too slow\n"))
				default:
					fctx.Write([]byte("Canceled\n"))
				}
				return

			case <-time.After(5 * time.Second):
				// The above channel simulates some hard work.
			}

			fctx.Write([]byte("Processed\n"))
		})
	})

	// RESTy routes for "articles" resource
	r.Route("/articles", func(r chi.Router) {
		r.Get("/", paginate, listArticles) // GET /articles
		r.Post("/", createArticle)         // POST /articles

		r.Route("/:articleID", func(r chi.Router) {
			r.Use(ArticleCtx)
			r.Get("/", getArticle)       // GET /articles/123
			r.Put("/", updateArticle)    // PUT /articles/123
			r.Delete("/", deleteArticle) // DELETE /articles/123
		})
	})

	// Mount the admin sub-router
	r.Mount("/admin", adminRouter())

	fasthttp.ListenAndServe(":3333", r.ServeHTTP)
}

type Article struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func ArticleCtx(next chi.Handler) chi.Handler {
	return chi.HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		articleID := chi.URLParam(ctx, "articleID")
		article, err := dbGetArticle(articleID)
		if err != nil {
			fctx.Error("Not Found", 404)
			return
		}
		ctx = context.WithValue(ctx, "article", article)
		next.ServeHTTPC(ctx, fctx)
	})
}

func listArticles(ctx context.Context, fctx *fasthttp.RequestCtx) {
	fctx.Write([]byte("list of articles.."))
	// or render.Data(w, 200, []byte("list of articles.."))
}

func createArticle(ctx context.Context, fctx *fasthttp.RequestCtx) {
	var article *Article

	// btw, you could do this body reading / marhsalling in a nice bind middleware
	data := fctx.PostBody()

	if err := json.Unmarshal(data, &article); err != nil {
		fctx.Error(err.Error(), 422)
		return
	}

	// should really send back the json marshalled new article.
	// build your own responder :)
	fctx.Write([]byte(article.Title))
}

func getArticle(ctx context.Context, fctx *fasthttp.RequestCtx) {
	article, ok := ctx.Value("article").(*Article)
	if !ok {
		fctx.Error("Unprocessable Entity", 422)
		return
	}

	// Build your own responder, see the "./render" pacakge as a starting
	// point for your own.
	render.JSON(fctx, 200, article)

	// or..
	// w.Write([]byte(fmt.Sprintf("title:%s", article.Title)))
}

func updateArticle(ctx context.Context, fctx *fasthttp.RequestCtx) {
	article, ok := ctx.Value("article").(*Article)
	if !ok {
		fctx.Error("Not Found", 404)
		return
	}

	// btw, you could do this body reading / marhsalling in a nice bind middleware
	data := fctx.PostBody()

	uArticle := struct {
		*Article
		_ interface{} `json:"id,omitempty"` // prevents 'id' from being overridden
	}{Article: article}

	if err := json.Unmarshal(data, &uArticle); err != nil {
		fctx.Error("Unprocessable Entity", 422)
		return
	}

	render.JSON(fctx, 200, uArticle)

	// w.Write([]byte(fmt.Sprintf("updated article, title:%s", uArticle.Title)))
}

func deleteArticle(ctx context.Context, fctx *fasthttp.RequestCtx) {
	article, ok := ctx.Value("article").(*Article)
	if !ok {
		fctx.Error("Unprocessable Entity", 422)
		return
	}
	_ = article // delete the article from the data store..
	fctx.SetStatusCode(204)
}

func dbGetArticle(id string) (*Article, error) {
	//.. fetch the article from a data store of some kind..
	return &Article{ID: id, Title: "Going all the way,"}, nil
}

func paginate(next chi.Handler) chi.Handler {
	return chi.HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		// just a stub.. some ideas are to look at URL query params for something like
		// the page number, or the limit, and send a query cursor down the chain
		next.ServeHTTPC(ctx, fctx)
	})
}

// A completely separate router for administrator routes
func adminRouter() chi.Handler { // or chi.Router {
	r := chi.NewRouter()
	r.Use(AdminOnly)
	r.Get("/", func(fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("admin: index"))
	})
	r.Get("/accounts", func(fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("admin: list accounts.."))
	})
	r.Get("/users/:userId", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte(fmt.Sprintf("admin: view user id %v", chi.URLParam(ctx, "userId"))))
	})
	return r
}

func AdminOnly(next chi.Handler) chi.Handler {
	return chi.HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		isAdmin, ok := ctx.Value("acl.admin").(bool)
		if !ok || !isAdmin {
			fctx.Error("Forbidden", 403)
			return
		}
		next.ServeHTTPC(ctx, fctx)
	})
}
