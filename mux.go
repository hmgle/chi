package chi

import (
	"fmt"
	"log"
	"net/http"

	"golang.org/x/net/context"
)

type Mux struct {
	middlewares []interface{}
	routes      map[methodTyp]*tree

	// can add rules here for how the mux should work..
	// ie. slashes, notfound handler etc.. like httprouter
}

type methodTyp int

const (
	mCONNECT methodTyp = 1 << iota
	mDELETE
	mGET
	mHEAD
	mOPTIONS
	mPATCH
	mPOST
	mPUT
	mTRACE

	// We only natively support the methods above, but we pass through other
	// methods. This constant pretty much only exists for the sake of mALL.
	mIDK // TODO: necessary?

	mALL methodTyp = mCONNECT | mDELETE | mGET | mHEAD | mOPTIONS | mPATCH |
		mPOST | mPUT | mTRACE | mIDK
)

var methodMap = map[string]methodTyp{
	"CONNECT": mCONNECT,
	"DELETE":  mDELETE,
	"GET":     mGET,
	"HEAD":    mHEAD,
	"OPTIONS": mOPTIONS,
	"PATCH":   mPATCH,
	"POST":    mPOST,
	"PUT":     mPUT,
	"TRACE":   mTRACE,
}

func (m methodTyp) String() string {
	for k, v := range methodMap {
		if v == m {
			return k
		}
	}
	return ""
}

type ctxKey int

const (
	urlParamsCtxKey ctxKey = iota
	subRouterCtxKey
)

func (mx *Mux) Use(mws ...interface{}) {
	for _, mw := range mws {
		switch t := mw.(type) {
		default:
			panic(fmt.Sprintf("chi: unsupported middleware signature: %T", t))
		case func(http.Handler) http.Handler:
		case func(Handler) Handler:
		}
		mx.middlewares = append(mx.middlewares, mw)
	}

	// switch t := mw.(type) {
	// default:
	// 	panic(fmt.Sprintf("chi: unsupported middleware signature: %T", t))
	// case func(http.Handler) http.Handler:
	// case func(Handler) Handler:
	// }
	// mx.middlewares = append(mx.middlewares, mw)
}

func (mx *Mux) Handle(pattern string, handlers ...interface{}) {
	mx.handle(mALL, pattern, handlers...)
}

func (mx *Mux) Connect(pattern string, handlers ...interface{}) {
	mx.handle(mCONNECT, pattern, handlers...)
}

func (mx *Mux) Head(pattern string, handlers ...interface{}) {
	mx.handle(mHEAD, pattern, handlers...)
}

func (mx *Mux) Get(pattern string, handlers ...interface{}) {
	mx.handle(mGET, pattern, handlers...)
}

func (mx *Mux) Post(pattern string, handlers ...interface{}) {
	mx.handle(mPOST, pattern, handlers...)
}

func (mx *Mux) Put(pattern string, handlers ...interface{}) {
	mx.handle(mPUT, pattern, handlers...)
}

func (mx *Mux) Patch(pattern string, handlers ...interface{}) {
	mx.handle(mPATCH, pattern, handlers...)
}

func (mx *Mux) Delete(pattern string, handlers ...interface{}) {
	mx.handle(mDELETE, pattern, handlers...)
}

func (mx *Mux) Trace(pattern string, handlers ...interface{}) {
	mx.handle(mTRACE, pattern, handlers...)
}

func (mx *Mux) Options(pattern string, handlers ...interface{}) {
	mx.handle(mOPTIONS, pattern, handlers...)
}

func (mx *Mux) handle(method methodTyp, pattern string, handlers ...interface{}) {
	// Build handler from middleware stack, inline middlewares and handler
	h := chain(mx.middlewares, handlers...)

	if pattern[0] != '/' {
		panic("pattern must begin with a /") // TODO: is goji like this too?
	}

	if mx.routes == nil {
		mx.routes = make(map[methodTyp]*tree, len(methodMap))
		for _, v := range methodMap {
			mx.routes[v] = &tree{root: &node{}}
		}
	}

	for _, mt := range methodMap {
		m := method & mt
		if m > 0 {
			routes := mx.routes[m]

			err := routes.Insert(pattern, h)
			_ = err // ...?
		}
	}
}

func (mx *Mux) Group(fn func(r Router)) Router {
	mw := make([]interface{}, len(mx.middlewares))
	copy(mw, mx.middlewares)

	g := &Mux{middlewares: mw, routes: mx.routes}
	if fn != nil {
		fn(g)
	}
	return g
}

func (mx *Mux) Route(pattern string, fn func(r Router)) Router {
	subRouter := NewRouter()
	mx.Mount(pattern, subRouter)
	if fn != nil {
		fn(subRouter)
	}
	return subRouter
}

func (mx *Mux) Mount(path string, handlers ...interface{}) {
	h := chain([]interface{}{}, handlers...)

	// subRouterIndex := HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// 	params := URLParams(ctx)
	// 	params["*"] = ""
	// 	ctx = context.WithValue(ctx, urlParamsCtxKey, params)
	// 	h.ServeHTTPC(ctx, w, r)
	// })
	// _ = subRouterIndex

	subRouter := HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		path := URLParams(ctx)["*"]

		xx := URLParams(ctx)["accountID"]
		log.Printf("====> subRouter path:'%s' xx:'%s' params:%v\n", path, xx, URLParams(ctx))

		ctx = context.WithValue(ctx, subRouterCtxKey, "/"+path)
		h.ServeHTTPC(ctx, w, r)
	})

	if path == "/" {
		path = ""
	}

	log.Printf("path is '%s'\n", path)

	// mx.Get(path, subRouter) // subRouterIndex ...? wrap .. set * to "" ....?
	mx.Handle(path, subRouter)
	if path != "" {
		mx.Handle(path+"/", http.NotFound) // TODO: which not-found handler..?
	}
	mx.Handle(path+"/*", subRouter)
}

func (mx *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mx.ServeHTTPC(context.Background(), w, r)
}

func (mx *Mux) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var cxh Handler
	var err error

	params, ok := ctx.Value(urlParamsCtxKey).(map[string]string) // ..?
	if !ok || params == nil {
		params = make(map[string]string, 0)
		ctx = context.WithValue(ctx, urlParamsCtxKey, params)
	}

	log.Println("")
	log.Println("")

	routes := mx.routes[methodMap[r.Method]]

	path := r.URL.Path
	if routePath, ok := ctx.Value(subRouterCtxKey).(string); ok {
		path = routePath
		ctx = context.WithValue(ctx, subRouterCtxKey, nil) // unset the routePath
		delete(params, "*")
	}

	log.Println("routePath:", path)
	cxh, err = routes.Find(path, params)
	_ = err // ..

	log.Println("********* CXH:", cxh)

	if cxh == nil {
		// not found..
		log.Println("** 404 **")
		w.WriteHeader(404)
		w.Write([]byte("~~ not found ~~"))
		return
	}

	// Serve it
	cxh.ServeHTTPC(ctx, w, r)
}