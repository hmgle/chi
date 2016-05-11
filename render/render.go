package render

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"reflect"

	"github.com/valyala/fasthttp"
)

func String(fctx *fasthttp.RequestCtx, status int, v string) {
	fctx.Response.Header.Set("Content-Type", "text/plain; charset=utf-8")
	fctx.SetStatusCode(status)
	fctx.Write([]byte(v))
}

func HTML(fctx *fasthttp.RequestCtx, status int, v string) {
	fctx.Response.Header.Set("Content-Type", "text/html; charset=utf-8")
	fctx.SetStatusCode(status)
	fctx.Write([]byte(v))
}

func JSON(fctx *fasthttp.RequestCtx, status int, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		fctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	if len(b) > 0 {
		b = bytes.Replace(b, []byte("\\u003c"), []byte("<"), -1)
		b = bytes.Replace(b, []byte("\\u003e"), []byte(">"), -1)
		b = bytes.Replace(b, []byte("\\u0026"), []byte("&"), -1)
	}

	fctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
	fctx.SetStatusCode(status)
	fctx.Write(b)
}

func Noop(fctx *fasthttp.RequestCtx) {
	String(fctx, fasthttp.StatusOK, "")
}

func XML(fctx *fasthttp.RequestCtx, status int, v interface{}) {
	b, err := xml.Marshal(v)
	if err != nil {
		fctx.Error(err.Error(), fasthttp.StatusInternalServerError)
		return
	}

	fctx.Response.Header.Set("Content-Type", "application/xml; charset=utf-8")
	fctx.SetStatusCode(status)

	// Try to find <?xml header in first 100 bytes (just in case there're some XML comments).
	findHeaderUntil := len(b)
	if findHeaderUntil > 100 {
		findHeaderUntil = 100
	}
	if bytes.Index(b[:findHeaderUntil], []byte("<?xml")) == -1 {
		// No header found. Print it out first.
		fctx.Write([]byte(xml.Header))
	}

	fctx.Write(b)
}

func Respond(fctx *fasthttp.RequestCtx, status int, v interface{}) {
	if err, ok := v.(error); ok {
		JSON(fctx, status, map[string]interface{}{"error": err.Error()})
		return
	}

	// Force to return empty JSON array [] instead of null in case of zero slice.
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Slice && val.IsNil() {
		v = reflect.MakeSlice(val.Type(), 0, 0).Interface()
	}

	JSON(fctx, status, v)
}
