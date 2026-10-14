// Package web is the HTTP boundary of an effectgo service: typed handlers
// over net/http, typed errors encoded as JSON, and the middleware that a
// service would otherwise write by hand (request ID, access log, body limit,
// Host check, token guard, security headers, static files).
//
// A handler is a function func(context.Context, In) (Out, error). In is
// decoded from the path (`path:"name"`), the query (`query:"name"`) and, for
// POST, PUT and PATCH, the JSON body; every other field is a body field.
// Decoding is done by package schema, so decode failures are a
// *schema.DecodeError with status 400. An error is encoded by its own type:
// an error implementing StatusCode() int is sent with that status and its
// exported JSON fields plus "_tag", the Go type name. An error without one is
// a 500 InternalError whose text is only logged, never sent.
//
// Handlers must be named functions or method values, not closures, so that
// the analyzer can read the error set of each route.
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/faintghost/effectgo/rt/schema"
)

// NoInput is the input of a handler that takes nothing. The body is not read.
type NoInput struct{}

// NoContent is the output of a handler that answers 204 with no body.
type NoContent struct{}

// InternalError is the error sent for any failure without a status. Its
// text is never sent.
type InternalError struct{}

// StatusCode reports 500.
func (InternalError) StatusCode() int { return http.StatusInternalServerError }

// Error describes the error for logs; the text is not sent to clients.
func (InternalError) Error() string { return "internal error" }

// BodyTooLarge is the answer to a request body over the limit: 413.
type BodyTooLarge struct{}

// StatusCode reports 413.
func (BodyTooLarge) StatusCode() int { return http.StatusRequestEntityTooLarge }

// Error describes the error.
func (BodyTooLarge) Error() string { return "request body too large" }

// Middleware wraps a handler. Middleware given to New wraps the whole router,
// in the order given: the first one is the outermost.
type Middleware func(http.Handler) http.Handler

// Option configures a Router.
type Option func(*Router)

// WithMiddleware appends middleware, outermost first.
func WithMiddleware(mw ...Middleware) Option {
	return func(r *Router) { r.mw = append(r.mw, mw...) }
}

// WithErrorEncoder replaces the default error encoding for handler, input
// and decode errors. The encoder receives the original error.
func WithErrorEncoder(fn func(w http.ResponseWriter, r *http.Request, err error)) Option {
	return func(r *Router) { r.encodeErr = fn }
}

// WithLogger sets the logger for internal errors, panics and failed response
// encoding. The default is slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(r *Router) { r.logger = l }
}

// WithPanicHandler sets a function that sees each panic that a handler or a
// middleware inside the router raises. It is called after the router logs the
// panic and before the router writes its 500 response, so it can count or report
// the panic and still see the response as not yet written. ctx is the request's
// context: RequestIDFrom and the values set by earlier middleware are there. The
// router's response does not change: a panic after the response has started is
// still re-raised, and the function does not see http.ErrAbortHandler, which
// net/http uses to abort a response on purpose. A panic in the function is
// logged and dropped. It panics when fn is nil.
func WithPanicHandler(fn func(ctx context.Context, r *http.Request, recovered any)) Option {
	if fn == nil {
		panic("web: WithPanicHandler(nil): the function must not be nil")
	}
	return func(r *Router) { r.panicFn = fn }
}

// Router serves typed routes. It implements http.Handler.
// WithMaxBody sets the most body bytes a route with a body input reads. A body
// over the limit is 413 BodyTooLarge, with or without BodyLimit middleware.
// The default is 1 MiB. It panics when n is not positive.
func WithMaxBody(n int64) Option {
	if n <= 0 {
		panic(fmt.Sprintf("web: WithMaxBody(%d): the limit must be positive", n))
	}
	return func(r *Router) { r.maxBody = n }
}

// DefaultMaxBody is the body limit of a router without WithMaxBody.
const DefaultMaxBody int64 = 1 << 20

// Router serves typed routes. It implements http.Handler.
type Router struct {
	mux       *http.ServeMux
	handler   http.Handler
	mw        []Middleware
	encodeErr func(w http.ResponseWriter, r *http.Request, err error)
	logger    *slog.Logger
	maxBody   int64
	panicFn   func(ctx context.Context, r *http.Request, recovered any)
}

// New returns a router with no routes.
func New(opts ...Option) *Router {
	r := &Router{mux: http.NewServeMux(), logger: slog.Default(), maxBody: DefaultMaxBody}
	for _, o := range opts {
		o(r)
	}
	var h http.Handler = http.HandlerFunc(r.serve)
	for i := len(r.mw) - 1; i >= 0; i-- {
		h = r.mw[i](h)
	}
	r.handler = h
	return r
}

// ServeHTTP implements http.Handler. It tracks the response at the outermost
// writer, so that a middleware that writes before the router runs is seen by
// the panic handling.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	tw := &trackWriter{ResponseWriter: w}
	req = req.WithContext(context.WithValue(req.Context(), trackKey{}, tw))
	r.handler.ServeHTTP(tw, req)
}

type trackKey struct{}

// serve is the innermost handler: it recovers panics and routes the request.
func (r *Router) serve(w http.ResponseWriter, req *http.Request) {
	tw, ok := req.Context().Value(trackKey{}).(*trackWriter)
	if !ok {
		tw = &trackWriter{ResponseWriter: w}
	}
	info, _ := req.Context().Value(routeKey{}).(*routeInfo)
	defer func() {
		if info != nil {
			info.route = req.Pattern
		}
		rec := recover()
		if rec == nil {
			return
		}
		if rec == http.ErrAbortHandler {
			panic(rec)
		}
		r.logger.LogAttrs(req.Context(), slog.LevelError, "handler panic",
			slog.String("request_id", RequestIDFrom(req.Context())),
			slog.String("route", req.Pattern),
			slog.Any("panic", rec),
			slog.String("stack", string(debug.Stack())),
		)
		r.notifyPanic(req, rec)
		if tw.wrote {
			// The response has started: the only way to abort it is to let
			// the panic reach net/http, as novastar's middleware does.
			panic(rec)
		}
		r.encodeError(w, req, InternalError{})
	}()
	r.mux.ServeHTTP(w, req)
}

// notifyPanic calls the panic handler of WithPanicHandler, if there is one. A
// panic in the handler is logged here, so it neither replaces the router's
// response nor escapes the request.
func (r *Router) notifyPanic(req *http.Request, rec any) {
	if r.panicFn == nil {
		return
	}
	defer func() {
		if p := recover(); p != nil {
			r.logger.LogAttrs(req.Context(), slog.LevelError, "panic handler panic",
				slog.String("request_id", RequestIDFrom(req.Context())),
				slog.String("route", req.Pattern),
				slog.Any("panic", p),
			)
		}
	}()
	r.panicFn(req.Context(), req, rec)
}

// encodeError writes err with the configured encoder, or the default one.
// An error without a valid status is logged here, with its request ID.
func (r *Router) encodeError(w http.ResponseWriter, req *http.Request, err error) {
	if _, ok := statusOf(err); !ok {
		r.logger.LogAttrs(req.Context(), slog.LevelError, "request failed",
			slog.String("request_id", RequestIDFrom(req.Context())),
			slog.String("route", req.Pattern),
			slog.String("error", err.Error()),
		)
	}
	if r.encodeErr != nil {
		r.encodeErr(w, req, err)
		return
	}
	status, body := ErrorBody(err)
	writeJSON(w, status, body)
}

// statusError is an error that carries its HTTP status.
type statusError interface {
	error
	StatusCode() int
}

// statusOf returns the status of the first error in err's chain that has a
// StatusCode method, provided the status is an error status (400 to 599).
func statusOf(err error) (int, bool) {
	var se statusError
	if !errors.As(err, &se) {
		return 0, false
	}
	status := se.StatusCode()
	if status < 400 || status > 599 {
		return 0, false
	}
	return status, true
}

// ErrorBody is the default error encoding: the status and the JSON body of
// err. The body is the exported JSON fields of the first error in err's chain
// with a StatusCode method, plus "_tag" set to the Go type name without
// pointers. An error without one, or with a status outside 400 to 599, gives
// 500 and {"_tag":"InternalError"}.
//
// Custom encoders (WithErrorEncoder) can reuse it and then rename or add
// fields.
func ErrorBody(err error) (status int, body map[string]any) {
	internal := func() (int, map[string]any) {
		return http.StatusInternalServerError, map[string]any{"_tag": "InternalError"}
	}
	var se statusError
	if !errors.As(err, &se) {
		return internal()
	}
	status, ok := statusOf(err)
	if !ok {
		return internal()
	}
	body = map[string]any{}
	if raw, mErr := json.Marshal(se); mErr == nil {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var fields map[string]any
		if dec.Decode(&fields) == nil && fields != nil {
			body = fields
		}
	}
	body["_tag"] = typeName(se)
	return status, body
}

func typeName(v any) string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() == "" {
		return "Error"
	}
	return t.Name()
}

// writeJSON writes a compact JSON body with no HTML escaping, like novastar's
// writeJSON, and without the trailing newline json.Encoder adds.
func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := encodeJSON(&buf, v); err != nil {
		status = http.StatusInternalServerError
		buf.Reset()
		buf.WriteString(`{"_tag":"InternalError"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func encodeJSON(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1)
	return nil
}

// routeKey and routeInfo let AccessLog learn the matched pattern, which the
// mux sets on the request it serves, a copy of the request AccessLog sees.
type routeKey struct{}

type routeInfo struct{ route string }

// trackWriter records whether a header or body has been written, so that a
// panic knows whether it can still answer with an error.
type trackWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *trackWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *trackWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *trackWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Get registers a GET route. See the package comment for the input rules.
func Get[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error)) {
	register(r, http.MethodGet, pattern, h)
}

// Post registers a POST route; the body is read.
func Post[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error)) {
	register(r, http.MethodPost, pattern, h)
}

// Put registers a PUT route; the body is read.
func Put[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error)) {
	register(r, http.MethodPut, pattern, h)
}

// Patch registers a PATCH route; the body is read.
func Patch[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error)) {
	register(r, http.MethodPatch, pattern, h)
}

// Delete registers a DELETE route; the body is not read.
func Delete[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error)) {
	register(r, http.MethodDelete, pattern, h)
}

// register checks the input and output types and the pattern, then adds the
// route. Every problem found here panics at startup.
func register[In, Out any](r *Router, method, pattern string, h func(context.Context, In) (Out, error)) {
	if h == nil {
		panic(fmt.Sprintf("web: %s %s: nil handler", method, pattern))
	}
	if err := schema.Compile[Out](); err != nil {
		panic(fmt.Sprintf("web: %s %s: output: %v", method, pattern, err))
	}
	in, err := newInputPlan[In](method, pattern)
	if err != nil {
		panic(fmt.Sprintf("web: %s %s: %v", method, pattern, err))
	}
	r.mux.HandleFunc(method+" "+pattern, func(w http.ResponseWriter, req *http.Request) {
		v, err := in.decode(w, req, r.maxBody)
		if err != nil {
			r.encodeError(w, req, err)
			return
		}
		out, err := h(req.Context(), v)
		if err != nil {
			r.encodeError(w, req, err)
			return
		}
		if reflect.TypeFor[Out]() == reflect.TypeFor[NoContent]() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var buf bytes.Buffer
		if err := encodeJSON(&buf, out); err != nil {
			r.logger.LogAttrs(req.Context(), slog.LevelError, "encode response failed",
				slog.String("request_id", RequestIDFrom(req.Context())),
				slog.String("route", req.Pattern),
				slog.String("error", err.Error()),
			)
			r.encodeError(w, req, InternalError{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	})
}

// readBody reads at most limit bytes. The limit is enforced by
// http.MaxBytesReader, so the read never buffers more than limit+1 bytes. A
// body over the limit is BodyTooLarge; any other read failure is a 400
// DecodeError.
func readBody(w http.ResponseWriter, req *http.Request, limit int64) ([]byte, error) {
	req.Body = http.MaxBytesReader(w, req.Body, limit)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, BodyTooLarge{}
		}
		return nil, &schema.DecodeError{Issues: []schema.Issue{{
			Path: []string{}, Code: schema.CodeSyntax, Message: "cannot read request body",
		}}}
	}
	return body, nil
}

// bodyObject parses a request body that must be a JSON object, or empty.
func bodyObject(body []byte) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	if !json.Valid(trimmed) {
		var v any
		err := json.Unmarshal(trimmed, &v)
		return nil, &schema.DecodeError{Issues: []schema.Issue{{Path: []string{}, Code: schema.CodeSyntax, Message: err.Error()}}}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil || obj == nil {
		return nil, &schema.DecodeError{Issues: []schema.Issue{{Path: []string{}, Code: schema.CodeType, Message: "expected object, got " + jsonWord(trimmed)}}}
	}
	return obj, nil
}

func jsonWord(raw []byte) string {
	switch raw[0] {
	case '"':
		return "string"
	case '[':
		return "array"
	case 'n':
		return "null"
	case 't', 'f':
		return "boolean"
	}
	return "number"
}

// nameOfPattern lists the wildcard names of a pattern, without "...".
func nameOfPattern(pattern string) []string {
	var names []string
	for _, seg := range strings.Split(pattern, "/") {
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") {
			continue
		}
		name := strings.TrimSuffix(seg[1:len(seg)-1], "...")
		if name != "$" {
			names = append(names, name)
		}
	}
	return names
}
