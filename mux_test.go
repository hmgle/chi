package chi

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"

	"github.com/valyala/fasthttp"

	"golang.org/x/net/context"
)

type readWriter struct {
	net.Conn
	r bytes.Buffer
	w bytes.Buffer
}

var zeroTCPAddr = &net.TCPAddr{
	IP: net.IPv4zero,
}

func (rw *readWriter) Close() error {
	return nil
}

func (rw *readWriter) Read(b []byte) (int, error) {
	return rw.r.Read(b)
}

func (rw *readWriter) Write(b []byte) (int, error) {
	return rw.w.Write(b)
}

func (rw *readWriter) RemoteAddr() net.Addr {
	return zeroTCPAddr
}

func (rw *readWriter) LocalAddr() net.Addr {
	return zeroTCPAddr
}

func (rw *readWriter) SetReadDeadline(t time.Time) error {
	return nil
}

func (rw *readWriter) SetWriteDeadline(t time.Time) error {
	return nil
}

func TestMuxServeHTTP(t *testing.T) {
	r := NewRouter()
	r.Get("/hi", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("bye"))
	})
	r.NotFound(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(404)
		fctx.Write([]byte("nothing here"))
	})

	// Thanks to https://github.com/mrcpvn for the nice table test code
	testcases := []struct {
		Method         string
		Path           string
		ExpectedStatus int
		ExpectedBody   string
	}{
		{
			Method:         "GET",
			Path:           "/hi",
			ExpectedStatus: 200,
			ExpectedBody:   "bye",
		},
		{
			Method:         "GET",
			Path:           "/hello",
			ExpectedStatus: 404,
			ExpectedBody:   "nothing here",
		},
	}

	for _, tc := range testcases {
		s := &fasthttp.Server{
			Handler: r.ServeHTTP,
		}
		rw := &readWriter{}
		ch := make(chan error)

		rw.r.WriteString(tc.Method + " " + tc.Path + " HTTP/1.1\r\n\r\n")
		go func() {
			ch <- s.ServeConn(rw)
		}()
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("return error %s", err)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("timeout")
		}
		br := bufio.NewReader(&rw.w)
		var resp fasthttp.Response
		if err := resp.Read(br); err != nil {
			t.Fatalf("Unexpected error when reading response: %s", err)
		}
		if resp.Header.StatusCode() != tc.ExpectedStatus {
			t.Fatalf("%v != %v", tc.ExpectedStatus, resp.Header.StatusCode())
		}
		if !bytes.Equal(resp.Body(), []byte(tc.ExpectedBody)) {
			t.Fatalf("%s != %s", tc.ExpectedBody, string(resp.Body()))
		}
	}
}

func TestMux(t *testing.T) {
	var count uint64
	countermw := func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			count++
			next.ServeHTTPC(ctx, fctx)
		})
	}

	usermw := func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			ctx = context.WithValue(ctx, "user", "peter")
			next.ServeHTTPC(ctx, fctx)
		})
	}

	exmw := func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			ctx = context.WithValue(ctx, "ex", "a")
			next.ServeHTTPC(ctx, fctx)
		})
	}
	_ = exmw

	logbuf := bytes.NewBufferString("")
	logmsg := "logmw test"
	logmw := func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			logbuf.WriteString(logmsg)
			next.ServeHTTPC(ctx, fctx)
		})
	}
	_ = logmw

	cxindex := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		user := ctx.Value("user").(string)
		fctx.SetStatusCode(200)
		fctx.Write([]byte(fmt.Sprintf("hi %s", user)))
	}

	ping := func(fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(200)
		fctx.Write([]byte("."))
	}

	headPing := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Response.Header.Set("X-Ping", "1")
		fctx.SetStatusCode(200)
	}

	createPing := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		// create ....
		fctx.SetStatusCode(201)
	}

	pingAll := func(fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(200)
		fctx.Write([]byte("ping all"))
	}
	_ = pingAll

	pingAll2 := func(fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(200)
		fctx.Write([]byte("ping all2"))
	}
	_ = pingAll2

	pingOne := func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		idParam := URLParam(ctx, "id")

		fctx.SetStatusCode(200)
		fctx.Write([]byte(fmt.Sprintf("ping one id: %s", idParam)))
	}

	pingWoop := func(fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(200)
		fctx.Write([]byte("woop."))
	}
	_ = pingWoop

	catchAll := func(fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(200)
		fctx.Write([]byte("catchall"))
	}
	_ = catchAll

	m := NewRouter()
	m.Use(countermw)
	m.Use(usermw)
	m.Use(exmw)
	m.Use(logmw)
	m.Get("/", cxindex)
	m.Get("/ping", ping)
	m.Get("/pingall", pingAll) // .. TODO: pingAll, case-sensitivity .. etc....?
	m.Get("/ping/all", pingAll)
	m.Get("/ping/all2", pingAll2)

	m.Head("/ping", headPing)
	m.Post("/ping", createPing)
	m.Get("/ping/:id", pingOne)
	m.Get("/ping/:id", pingOne) // should overwrite.. and just be 1
	m.Get("/ping/:id/woop", pingWoop)
	m.Handle("/admin/*", catchAll)
	// m.Post("/admin/*", catchAll)

	ts := &fasthttp.Server{
		Handler: m.ServeHTTP,
	}

	// GET /
	if resp := testRequest(t, ts, "GET", "/"); resp != "hi peter" {
		t.Fatalf(resp)
	}
	tlogmsg, _ := logbuf.ReadString(0)
	if tlogmsg != logmsg {
		t.Error("expecting log message from middlware:", logmsg)
	}

	// GET /ping
	if resp := testRequest(t, ts, "GET", "/ping"); resp != "." {
		t.Fatalf(resp)
	}

	// GET /pingall
	if resp := testRequest(t, ts, "GET", "/pingall"); resp != "ping all" {
		t.Fatalf(resp)
	}

	// GET /ping/all
	if resp := testRequest(t, ts, "GET", "/ping/all"); resp != "ping all" {
		t.Fatalf(resp)
	}

	// GET /ping/all2
	if resp := testRequest(t, ts, "GET", "/ping/all2"); resp != "ping all2" {
		t.Fatalf(resp)
	}

	// GET /ping/123
	if resp := testRequest(t, ts, "GET", "/ping/123"); resp != "ping one id: 123" {
		t.Fatalf(resp)
	}

	// GET /ping/allan
	if resp := testRequest(t, ts, "GET", "/ping/allan"); resp != "ping one id: allan" {
		t.Fatalf(resp)
	}

	// GET /ping/1/woop
	if resp := testRequest(t, ts, "GET", "/ping/1/woop"); resp != "woop." {
		t.Fatalf(resp)
	}

	// HEAD /ping
	rw := &readWriter{}
	ch := make(chan error)
	rw.r.WriteString("HEAD" + " " + "/ping" + " HTTP/1.1\r\n\r\n")
	go func() {
		ch <- ts.ServeConn(rw)
	}()
	select {
	case err := <-ch:
		if err != nil {
			t.Fatalf("return error %s", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout")
	}
	br := bufio.NewReader(&rw.w)
	var resp fasthttp.Response
	if err := resp.Read(br); err != nil {
		t.Fatalf("Unexpected error when reading response: %s", err)
	}
	if resp.Header.StatusCode() != 200 {
		t.Error("head failed, should be 200")
	}
	if string(resp.Header.Peek("X-Ping")) == "" {
		t.Error("expecting X-Ping header")
	}

	// GET /admin/catch-this
	if resp := testRequest(t, ts, "GET", "/admin/catch-thazzzzz"); resp != "catchall" {
		t.Fatalf(resp)
	}

	// TODO: POST /admin/catch-this
	// resp, err = http.Post(ts.URL+"/admin/casdfsadfs", "text/plain", bytes.NewReader([]byte{}))
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// body, err := ioutil.ReadAll(resp.Body)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// defer resp.Body.Close()

	// if resp.StatusCode != 200 {
	// 	t.Error("POST failed, should be 200")
	// }

	// if string(body) != "catchall" {
	// 	t.Error("expecting response body: 'catchall'")
	// }

	// Custom http method DIE /ping/1/woop
	if resp := testRequest(t, ts, "DIE", "/ping/1/woop"); resp != "Method Not Allowed" {
		t.Fatalf(resp)
	}
}

func TestMuxPlain(t *testing.T) {
	r := NewRouter()
	r.Get("/hi", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("bye"))
	})
	r.NotFound(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(404)
		fctx.Write([]byte("nothing here"))
	})

	ts := &fasthttp.Server{
		Handler: r.ServeHTTP,
	}

	if resp := testRequest(t, ts, "GET", "/hi"); resp != "bye" {
		t.Fatalf(resp)
	}
	if resp := testRequest(t, ts, "GET", "/nothing-here"); resp != "nothing here" {
		t.Fatalf(resp)
	}
}

func TestMuxNestedNotFound(t *testing.T) {
	r := NewRouter()
	r.Get("/hi", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("bye"))
	})
	r.NotFound(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(404)
		fctx.Write([]byte("root 404"))
	})

	sr1 := NewRouter()
	sr1.Get("/sub", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("sub"))
	})
	sr1.NotFound(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.SetStatusCode(404)
		fctx.Write([]byte("sub 404"))
	})

	sr2 := NewRouter()
	sr2.Get("/sub", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("sub2"))
	})

	r.Mount("/admin1", sr1)
	r.Mount("/admin2", sr2)

	ts := &fasthttp.Server{
		Handler: r.ServeHTTP,
	}

	if resp := testRequest(t, ts, "GET", "/hi"); resp != "bye" {
		t.Fatalf(resp)
	}
	if resp := testRequest(t, ts, "GET", "/nothing-here"); resp != "root 404" {
		t.Fatalf(resp)
	}
	if resp := testRequest(t, ts, "GET", "/admin1/sub"); resp != "sub" {
		t.Fatalf(resp)
	}
	if resp := testRequest(t, ts, "GET", "/admin1/nope"); resp != "sub 404" {
		t.Fatalf(resp)
	}
	if resp := testRequest(t, ts, "GET", "/admin2/sub"); resp != "sub2" {
		t.Fatalf(resp)
	}

	// Not found pages should bubble up to the root.
	if resp := testRequest(t, ts, "GET", "/admin2/nope"); resp != "root 404" {
		t.Fatalf(resp)
	}

}

func TestMuxMiddlewareStack(t *testing.T) {
	var stdmwInit, stdmwHandler uint64
	stdmw := func(next Handler) Handler {
		stdmwInit++
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			stdmwHandler++
			next.ServeHTTPC(ctx, fctx)
		})
	}
	_ = stdmw

	var ctxmwInit, ctxmwHandler uint64
	ctxmw := func(next Handler) Handler {
		ctxmwInit++
		// log.Println("INIT")
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			ctxmwHandler++
			ctx = context.WithValue(ctx, "count.ctxmwHandler", ctxmwHandler)
			next.ServeHTTPC(ctx, fctx)
		})
	}

	var inCtxmwInit, inCtxmwHandler uint64
	inCtxmw := func(next Handler) Handler {
		inCtxmwInit++
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			inCtxmwHandler++
			next.ServeHTTPC(ctx, fctx)
		})
	}

	r := NewRouter()
	r.Use(stdmw)
	r.Use(ctxmw)
	r.Use(func(next Handler) Handler {
		// log.Println("std, inline mw init")
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			next.ServeHTTPC(ctx, fctx)
		})
	})
	// r.Use(func(next http.Handler) http.Handler {
	// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 		next.ServeHTTP(w, r)
	// 	})
	// })
	r.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			if string(fctx.Path()) == "/ping" {
				fctx.Write([]byte("pong"))
				return
			}
			next.ServeHTTPC(ctx, fctx)
		})
	})

	var handlerCount uint64
	r.Get("/", inCtxmw, func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		handlerCount++
		ctxmwHandlerCount := ctx.Value("count.ctxmwHandler").(uint64)
		fctx.Write([]byte(fmt.Sprintf("inits:%d reqs:%d ctxValue:%d", ctxmwInit, handlerCount, ctxmwHandlerCount)))
	})

	r.Get("/hi", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("wooot"))
	})

	ts := &fasthttp.Server{
		Handler: r.ServeHTTP,
	}

	// log.Println("routes set.")

	var resp string
	resp = testRequest(t, ts, "GET", "/")
	resp = testRequest(t, ts, "GET", "/")
	resp = testRequest(t, ts, "GET", "/")
	if resp != "inits:1 reqs:3 ctxValue:3" {
		t.Fatalf("got: '%s'", resp)
	}

	resp = testRequest(t, ts, "GET", "/ping")
	if resp != "pong" {
		t.Fatalf("got: '%s'", resp)
	}
}

func TestMuxRootGroup(t *testing.T) {
	var stdmwInit, stdmwHandler uint64
	stdmw := func(next Handler) Handler {
		stdmwInit++
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			// log.Println("$$$$$ stdmw handlerfunc here!")
			stdmwHandler++
			next.ServeHTTPC(ctx, fctx)
		})
	}
	// stdmw := func(next Handler) Handler {
	// 	stdmwInit++
	// 	return HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// 		log.Println("$$$$$ stdmw handlerfunc here!")
	// 		stdmwHandler++
	// 		next.ServeHTTPC(ctx, w, r)
	// 	})
	// }

	r := NewRouter()
	// r.Use(func(next Handler) Handler {
	// 	return HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// 		next.ServeHTTPC(ctx, w, r)
	// 	})
	// })
	r.Group(func(r Router) {
		r.Use(stdmw)
		r.Get("/group", func(fctx *fasthttp.RequestCtx) {
			fctx.Write([]byte("root group"))
		})
	})

	ts := &fasthttp.Server{
		Handler: r.ServeHTTP,
	}

	// GET /group
	resp := testRequest(t, ts, "GET", "/group")
	if resp != "root group" {
		t.Fatalf("got: '%s'", resp)
	}
	if stdmwInit != 1 || stdmwHandler != 1 {
		t.Fatalf("stdmw counters failed, should be 1:1, got %d:%d", stdmwInit, stdmwHandler)
	}
}

func TestMuxBig(t *testing.T) {
	var r, sr1, sr2, sr3, sr4, sr5, sr6 *Mux
	r = NewRouter()
	r.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			ctx = context.WithValue(ctx, "requestID", "1")
			next.ServeHTTPC(ctx, fctx)
		})
	})
	r.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			// log.Println("request:", r.URL.Path) // TODO: put in buffer..
			next.ServeHTTPC(ctx, fctx)
		})
	})
	r.Group(func(r Router) {
		r.Use(func(next Handler) Handler {
			return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
				next.ServeHTTPC(ctx, fctx)
			})
		})
		r.Get("/favicon.ico", func(fctx *fasthttp.RequestCtx) {
			fctx.Write([]byte("fav"))
		})
		r.Get("/hubs/:hubID/view", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			s := fmt.Sprintf("/hubs/%s/view reqid:%s", URLParam(ctx, "hubID"), ctx.Value("requestID"))
			fctx.Write([]byte(s))
		})
		r.Get("/hubs/:hubID/view/*", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			s := fmt.Sprintf("/hubs/%s/view/%s reqid:%s", URLParam(ctx, "hubID"), URLParam(ctx, "*"),
				ctx.Value("requestID"))
			fctx.Write([]byte(s))
		})
	})
	r.Group(func(r Router) {
		r.Use(func(next Handler) Handler {
			return HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
				ctx = context.WithValue(ctx, "session.user", "elvis")
				next.ServeHTTPC(ctx, fctx)
			})
		})
		r.Get("/", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			s := fmt.Sprintf("/ reqid:%s session:%s", ctx.Value("requestID"), ctx.Value("session.user"))
			fctx.Write([]byte(s))
		})
		r.Get("/suggestions", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			s := fmt.Sprintf("/suggestions reqid:%s session:%s", ctx.Value("requestID"), ctx.Value("session.user"))
			fctx.Write([]byte(s))
		})

		r.Get("/woot/:wootID/*", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
			s := fmt.Sprintf("/woot/%s/%s", URLParam(ctx, "wootID"), URLParam(ctx, "*"))
			fctx.Write([]byte(s))
		})

		r.Route("/hubs", func(r Router) {
			sr1 = r.(*Mux)
			r.Route("/:hubID", func(r Router) {
				sr2 = r.(*Mux)
				r.Get("/", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
					s := fmt.Sprintf("/hubs/%s reqid:%s session:%s",
						URLParam(ctx, "hubID"), ctx.Value("requestID"), ctx.Value("session.user"))
					fctx.Write([]byte(s))
				})
				r.Get("/touch", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
					s := fmt.Sprintf("/hubs/%s/touch reqid:%s session:%s", URLParam(ctx, "hubID"),
						ctx.Value("requestID"), ctx.Value("session.user"))
					fctx.Write([]byte(s))
				})

				sr3 = NewRouter()
				sr3.Get("/", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
					s := fmt.Sprintf("/hubs/%s/webhooks reqid:%s session:%s", URLParam(ctx, "hubID"),
						ctx.Value("requestID"), ctx.Value("session.user"))
					fctx.Write([]byte(s))
				})
				sr3.Route("/:webhookID", func(r Router) {
					sr4 = r.(*Mux)
					r.Get("/", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
						s := fmt.Sprintf("/hubs/%s/webhooks/%s reqid:%s session:%s", URLParam(ctx, "hubID"),
							URLParam(ctx, "webhookID"), ctx.Value("requestID"), ctx.Value("session.user"))
						fctx.Write([]byte(s))
					})
				})
				r.Mount("/webhooks", sr3)

				r.Route("/posts", func(r Router) {
					sr5 = r.(*Mux)
					r.Get("/", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
						s := fmt.Sprintf("/hubs/%s/posts reqid:%s session:%s", URLParam(ctx, "hubID"),
							ctx.Value("requestID"), ctx.Value("session.user"))
						fctx.Write([]byte(s))
					})
				})
			})
		})

		r.Route("/folders/", func(r Router) {
			sr6 = r.(*Mux)
			r.Get("/", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
				s := fmt.Sprintf("/folders/ reqid:%s session:%s",
					ctx.Value("requestID"), ctx.Value("session.user"))
				fctx.Write([]byte(s))
			})
			r.Get("/public", func(ctx context.Context, fctx *fasthttp.RequestCtx) {
				s := fmt.Sprintf("/folders/public reqid:%s session:%s",
					ctx.Value("requestID"), ctx.Value("session.user"))
				fctx.Write([]byte(s))
			})
		})
	})

	// log.Println("")
	// log.Println("~~router")
	// debugPrintTree(0, 0, r.router[mGET].root, 0)
	// log.Println("")
	// log.Println("")
	//
	// log.Println("~~subrouter1")
	// debugPrintTree(0, 0, sr1.router[mGET].root, 0)
	// log.Println("")
	//
	// log.Println("~~subrouter2")
	// debugPrintTree(0, 0, sr2.router[mGET].root, 0)
	// log.Println("")
	//
	// log.Println("~~subrouter3")
	// debugPrintTree(0, 0, sr3.router[mGET].root, 0)
	// log.Println("")
	//
	// log.Println("~~subrouter4")
	// debugPrintTree(0, 0, sr4.router[mGET].root, 0)
	// log.Println("")
	//
	// log.Println("~~subrouter5")
	// debugPrintTree(0, 0, sr5.router[mGET].root, 0)
	// log.Println("")
	//
	// log.Println("~~subrouter6")
	// debugPrintTree(0, 0, sr6.router[mGET].root, 0)
	// log.Println("")

	ts := &fasthttp.Server{
		Handler: r.ServeHTTP,
	}

	var resp, expected string

	resp = testRequest(t, ts, "GET", "/favicon.ico")
	if resp != "fav" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/hubs/4/view")
	if resp != "/hubs/4/view reqid:1" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/hubs/4/view/index.html")
	if resp != "/hubs/4/view/index.html reqid:1" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/")
	if resp != "/ reqid:1 session:elvis" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/suggestions")
	if resp != "/suggestions reqid:1 session:elvis" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/woot/444/hiiii")
	if resp != "/woot/444/hiiii" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/hubs/123")
	expected = "/hubs/123 reqid:1 session:elvis"
	if resp != expected {
		t.Fatalf("expected:%s got:%s", expected, resp)
	}
	resp = testRequest(t, ts, "GET", "/hubs/123/touch")
	if resp != "/hubs/123/touch reqid:1 session:elvis" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/hubs/123/webhooks")
	if resp != "/hubs/123/webhooks reqid:1 session:elvis" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/hubs/123/posts")
	if resp != "/hubs/123/posts reqid:1 session:elvis" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/folders")
	if resp != "404 Page not found" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/folders/")
	if resp != "/folders/ reqid:1 session:elvis" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/folders/public")
	if resp != "/folders/public reqid:1 session:elvis" {
		t.Fatalf("got '%s'", resp)
	}
	resp = testRequest(t, ts, "GET", "/folders/nothing")
	if resp != "404 Page not found" {
		t.Fatalf("got '%s'", resp)
	}
}

func TestMuxSubroutes(t *testing.T) {
	hHubView1 := HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("hub1"))
	})
	hHubView2 := HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("hub2"))
	})
	hHubView3 := HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("hub3"))
	})
	hAccountView1 := HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("account1"))
	})
	hAccountView2 := HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		fctx.Write([]byte("account2"))
	})

	r := NewRouter()
	r.Get("/hubs/:hubID/view", hHubView1)
	r.Get("/hubs/:hubID/view/*", hHubView2)

	sr := NewRouter()
	sr.Get("/", hHubView3)
	r.Mount("/hubs/:hubID/users", sr)

	sr3 := NewRouter()
	sr3.Get("/", hAccountView1)
	sr3.Get("/hi", hAccountView2)

	var sr2 *Mux
	r.Route("/accounts/:accountID", func(r Router) {
		sr2 = r.(*Mux)
		r.Mount("/", sr3)
	})

	// TODO: support overriding the index method on a mount like:
	// r.Get("/users", UIndex)
	// r.Mount("/users", U) // assuming U router doesn't implement index route
	// .. currently for this to work, the index route must be defined separately

	// log.Println("")
	// log.Println("~~router:")
	// debugPrintTree(0, 0, r.router[mGET].root, 0)
	//
	// log.Println("")
	// log.Println("~~subrouter1:")
	// debugPrintTree(0, 0, sr.router[mGET].root, 0)
	// log.Println("")
	// log.Println("")
	//
	// log.Println("")
	// log.Println("~~subrouter2:")
	// debugPrintTree(0, 0, sr2.router[mGET].root, 0)
	// log.Println("")
	// log.Println("")
	//
	// log.Println("")
	// log.Println("~~subrouter3:")
	// debugPrintTree(0, 0, sr3.router[mGET].root, 0)
	// log.Println("")
	// log.Println("")

	ts := &fasthttp.Server{
		Handler: r.ServeHTTP,
	}

	var resp, expected string

	resp = testRequest(t, ts, "GET", "/hubs/123/view")
	expected = "hub1"
	if resp != expected {
		t.Fatalf("expected:%s got:%s", expected, resp)
	}
	resp = testRequest(t, ts, "GET", "/hubs/123/view/index.html")
	expected = "hub2"
	if resp != expected {
		t.Fatalf("expected:%s got:%s", expected, resp)
	}
	resp = testRequest(t, ts, "GET", "/hubs/123/users")
	expected = "hub3"
	if resp != expected {
		t.Fatalf("expected:%s got:%s", expected, resp)
	}
	resp = testRequest(t, ts, "GET", "/accounts/44")
	expected = "account1"
	if resp != expected {
		t.Fatalf("request:%s expected:%s got:%s", "GET /accounts/44", expected, resp)
	}
	resp = testRequest(t, ts, "GET", "/accounts/44/hi")
	expected = "account2"
	if resp != expected {
		t.Fatalf("expected:%s got:%s", expected, resp)
	}
}

func catchPanic(testFunc func()) (recv interface{}) {
	defer func() {
		recv = recover()
	}()

	testFunc()
	return
}

func TestMuxFileServer(t *testing.T) {
	r := NewRouter()

	recv := catchPanic(func() {
		r.FileServer("/noFilepath", os.TempDir())
	})
	if recv == nil {
		t.Fatal("registering path not ending with '*filepath' did not panic")
	}
	body := []byte("fake ico")
	ioutil.WriteFile(os.TempDir()+"/favicon.ico", body, 0644)

	r.FileServer("/*filepath", os.TempDir())

	ts := &fasthttp.Server{
		Handler: r.ServeHTTP,
	}

	rw := &readWriter{}
	ch := make(chan error)

	rw.r.WriteString(string("GET /favicon.ico HTTP/1.1\r\n\r\n"))
	go func() {
		ch <- ts.ServeConn(rw)
	}()
	select {
	case err := <-ch:
		if err != nil {
			t.Fatalf("return error %s", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout")
	}

	br := bufio.NewReader(&rw.w)
	var resp fasthttp.Response
	if err := resp.Read(br); err != nil {
		t.Fatalf("Unexpected error when reading response: %s", err)
	}
	if resp.Header.StatusCode() != 200 {
		t.Fatalf("Unexpected status code %d. Expected %d", resp.Header.StatusCode(), 423)
	}
	if !bytes.Equal(resp.Body(), body) {
		t.Fatalf("Unexpected body %q. Expected %q", resp.Body(), string(body))
	}
}

func urlParams(ctx context.Context) map[string]string {
	if rctx := RouteContext(ctx); rctx != nil {
		m := make(map[string]string, 0)
		for _, p := range rctx.Params {
			m[p.Key] = p.Value
		}
		return m
	}
	return nil
}

func testRequest(t *testing.T, ts *fasthttp.Server, method, path string) string {
	rw := &readWriter{}
	ch := make(chan error)

	rw.r.WriteString(method + " " + path + " HTTP/1.1\r\n\r\n")
	go func() {
		ch <- ts.ServeConn(rw)
	}()
	select {
	case err := <-ch:
		if err != nil {
			t.Fatal(err)
			return ""
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout")
		return ""
	}
	br := bufio.NewReader(&rw.w)
	var resp fasthttp.Response
	if err := resp.Read(br); err != nil {
		t.Fatal(err)
		return ""
	}
	return string(resp.Body())
}
