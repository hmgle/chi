package render

import (
	"strings"

	"github.com/hmgle/chi"
	"github.com/valyala/fasthttp"
	"golang.org/x/net/context"
)

// A ContentType is an enumeration of HTTP content types.
type ContentType int

const (
	ContentTypePlainText = iota
	ContentTypeHTML
	ContentTypeJSON
	ContentTypeEventStream
	ContentTypeXML
)

func ParseContentType(next chi.Handler) chi.Handler {
	return chi.HandlerFunc(func(ctx context.Context, fctx *fasthttp.RequestCtx) {
		var contentType ContentType

		// Parse request Accept header.
		fields := strings.Split(string(fctx.Request.Header.Peek("Accept")), ",")
		if len(fields) > 0 {
			switch strings.TrimSpace(fields[0]) {
			case "text/plain":
				contentType = ContentTypePlainText
			case "text/html", "application/xhtml+xml":
				contentType = ContentTypeHTML
			case "application/json", "text/javascript":
				contentType = ContentTypeJSON
			case "text/event-stream":
				contentType = ContentTypeEventStream
			case "text/xml":
				contentType = ContentTypeXML
			default:
				contentType = ContentTypeJSON
			}
		}

		// TODO
		// Explicitly requested stream.
		// if _, ok := r.URL.Query()["stream"]; ok {
		if fctx.URI().QueryArgs().Peek("stream") != nil {
			contentType = ContentTypeEventStream
		}

		ctx = context.WithValue(ctx, "contentType", contentType)
		next.ServeHTTPC(ctx, fctx)
	})
}
