package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type requestIDKey struct{}

// RequestIDFrom returns the request ID set by RequestID, or "".
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// writeTagged answers with a status and {"_tag": tag}. Middleware bodies use
// the same tagged shape as handler errors, but cannot be customised by
// WithErrorEncoder, which only sees handler errors.
func writeTagged(w http.ResponseWriter, status int, tag string) {
	writeJSON(w, status, map[string]any{"_tag": tag})
}

// RequestID reads X-Request-ID from the request when it is a short token
// (1 to 128 characters of letters, digits, '.', '_' and '-'), and generates one
// otherwise. The ID is written to the response header and kept in the
// context, where RequestIDFrom and the router's error logs find it.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if !validRequestID(id) {
				id = newRequestID()
			}
			w.Header().Set("X-Request-ID", id)
			ctx := context.WithValue(r.Context(), requestIDKey{}, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// statusWriter records the status the handler answers with, for AccessLog.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

var standardMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true, http.MethodOptions: true,
}

// AccessLog writes one JSON log line per request, after the handler returns:
// request_id, method, route (the matched pattern, empty when none matched),
// status, duration_ms. Bodies and query values are never logged. Methods
// outside the standard set are logged as OTHER. A nil logger is slog.Default().
// Put it inside RequestID so that the request ID is logged.
func AccessLog(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := &routeInfo{}
			r = r.WithContext(context.WithValue(r.Context(), routeKey{}, info))
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			started := time.Now()
			defer func() {
				method := r.Method
				if !standardMethods[method] {
					method = "OTHER"
				}
				level := slog.LevelInfo
				if sw.status >= 500 {
					level = slog.LevelError
				}
				logger.LogAttrs(r.Context(), level, "http_request",
					slog.String("request_id", RequestIDFrom(r.Context())),
					slog.String("method", method),
					slog.String("route", info.route),
					slog.Int("status", sw.status),
					slog.Float64("duration_ms", float64(time.Since(started).Microseconds())/1000),
				)
			}()
			next.ServeHTTP(sw, r)
		})
	}
}

// HostCheck answers 403 HostNotAllowed unless the Host header is exactly one
// of the allowed values. It panics when no value is given.
func HostCheck(allowed ...string) Middleware {
	if len(allowed) == 0 {
		panic("web: HostCheck needs at least one allowed host")
	}
	set := map[string]bool{}
	for _, h := range allowed {
		set[h] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !set[r.Host] {
				writeTagged(w, http.StatusForbidden, "HostNotAllowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TokenGuard requires header to equal token on every method except GET and
// HEAD; otherwise it answers 403 TokenMismatch. The comparison is constant
// time. It panics when token is empty, because an empty token would accept a
// request without the header.
func TokenGuard(header, token string) Middleware {
	if header == "" || token == "" {
		panic("web: TokenGuard needs a header name and a non-empty token")
	}
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			got := sha256.Sum256([]byte(r.Header.Get(header)))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				writeTagged(w, http.StatusForbidden, "TokenMismatch")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets Cache-Control: no-store, X-Content-Type-Options:
// nosniff and, when csp is not empty, Content-Security-Policy on every
// response, before the handler runs.
func SecurityHeaders(csp string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Cache-Control", "no-store")
			h.Set("X-Content-Type-Options", "nosniff")
			if csp != "" {
				h.Set("Content-Security-Policy", csp)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BodyLimit answers 413 BodyTooLarge when Content-Length is over n, and wraps
// the body so that a longer body fails to read with *http.MaxBytesError,
// which the router turns into the same 413. A Content-Length that is present
// but not a number is a 500 InternalError, as in novastar. It panics when n
// is negative.
func BodyLimit(n int64) Middleware {
	if n < 0 {
		panic("web: BodyLimit needs a non-negative limit")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cl := r.Header.Get("Content-Length"); cl != "" {
				length, err := strconv.ParseInt(cl, 10, 64)
				if err != nil {
					writeTagged(w, http.StatusInternalServerError, "InternalError")
					return
				}
				if length > n {
					writeTagged(w, http.StatusRequestEntityTooLarge, "BodyTooLarge")
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
