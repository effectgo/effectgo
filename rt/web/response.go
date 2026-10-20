package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Response supplies HTTP metadata around a concrete JSON body. Zero Status uses
// the route's SuccessStatus. Declare finite success alternatives on the route.
// A provider's error response may preserve its original status without being
// mistaken for a successful generated-client result.
type Response[T any] struct {
	Body   T
	Status int
	Header http.Header
}

func (r Response[T]) responseBody() any            { return r.Body }
func (r Response[T]) responseStatus() int          { return r.Status }
func (r Response[T]) responseHeaders() http.Header { return r.Header }
func (r Response[T]) responseProxy() bool          { return false }

// ProxyResponse separates a typed success body from a provider's failure body.
// Status zero sends Body with the route's SuccessStatus (200 by default).
// Any nonzero Status sends Failure with that exact HTTP status, including 2xx,
// and must be between 100 and 999 and differ from the route's success status.
// Bodyless HTTP responses omit the body. Use SuccessStatus for the single
// successful result.
type ProxyResponse[Success, Failure any] struct {
	Body    Success
	Failure Failure
	Status  int
	Header  http.Header
}

func (r ProxyResponse[Success, Failure]) responseBody() any {
	if r.Status != 0 {
		return r.Failure
	}
	return r.Body
}

func (r ProxyResponse[Success, Failure]) responseStatus() int          { return r.Status }
func (r ProxyResponse[Success, Failure]) responseHeaders() http.Header { return r.Header }
func (r ProxyResponse[Success, Failure]) responseProxy() bool          { return true }

type jsonResponse interface {
	responseBody() any
	responseStatus() int
	responseHeaders() http.Header
	responseProxy() bool
}

// FileResponse streams an owned body. The router closes Body, including on
// cancellation. Status zero means 200. Seekable 200 bodies retain net/http range
// and conditional GET semantics. Size is negative if unknown. Filename makes an
// attachment; Header may supply an inline disposition or cache/security headers.
type FileResponse struct {
	Body        io.ReadCloser
	ContentType string
	Filename    string
	Size        int64
	ModTime     time.Time
	Header      http.Header
	Status      int
}

// FileContent describes one callback-scoped stream. Size is negative if unknown.
type FileContent struct {
	ContentType string
	Filename    string
	Size        int64
	Header      http.Header
}

// StreamResponse preserves a provider whose reader only lives in its callback.
// Stream calls send once and returns its error. The provider owns reader cleanup;
// the router copies directly to the response without buffering the whole body.
type StreamResponse struct {
	Stream func(context.Context, func(FileContent, io.Reader) error) error
}

// RedirectResponse writes a net/http redirect, defaulting to 302.
type RedirectResponse struct{ Location string }

func redirectStatus(status int) bool {
	return status == http.StatusMovedPermanently || status == http.StatusFound || status == http.StatusSeeOther || status == http.StatusTemporaryRedirect || status == http.StatusPermanentRedirect
}

func setResponseHeaders(w http.ResponseWriter, headers http.Header) error {
	for name, values := range headers {
		for _, value := range values {
			if strings.ContainsAny(name+value, "\r\n") {
				return errors.New("web: invalid response header")
			}
		}
	}
	for name, values := range headers {
		w.Header()[http.CanonicalHeaderKey(name)] = values
	}
	return nil
}

func setFileHeaders(w http.ResponseWriter, content FileContent) error {
	if strings.ContainsAny(content.ContentType, "\r\n") {
		return errors.New("web: invalid file content type")
	}
	if err := setResponseHeaders(w, content.Header); err != nil {
		return err
	}
	if content.ContentType != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", content.ContentType)
	}
	if content.Filename != "" && w.Header().Get("Content-Disposition") == "" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": content.Filename}))
	}
	return nil
}

type responseContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r responseContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type responseContextSeeker struct {
	responseContextReader
	io.Seeker
}

func (r *Router) sendFile(w http.ResponseWriter, req *http.Request, out FileResponse) error {
	if out.Body == nil {
		return errors.New("web: FileResponse has no body")
	}
	closeBody := sync.OnceValue(out.Body.Close)
	stop := context.AfterFunc(req.Context(), func() { _ = closeBody() })
	defer stop()
	defer func() {
		if err := closeBody(); err != nil {
			r.logger.ErrorContext(req.Context(), "close response body failed", "error", err)
		}
	}()
	if err := req.Context().Err(); err != nil {
		return err
	}
	status := out.Status
	if status == 0 {
		status = http.StatusOK
	}
	if status < 200 || status > 299 {
		return errors.New("web: invalid file response status")
	}
	if status == http.StatusNoContent || status == http.StatusResetContent {
		if err := setResponseHeaders(w, out.Header); err != nil {
			return err
		}
		w.WriteHeader(status)
		return nil
	}
	content := FileContent{ContentType: out.ContentType, Filename: out.Filename, Size: out.Size, Header: out.Header}
	if body, ok := out.Body.(io.ReadSeeker); ok && status == http.StatusOK {
		if err := setFileHeaders(w, content); err != nil {
			return err
		}
		http.ServeContent(w, req, out.Filename, out.ModTime, responseContextSeeker{responseContextReader{req.Context(), body}, body})
		return req.Context().Err()
	}
	return sendStreamBody(w, req, status, content, out.Body)
}

func sendStreamBody(w http.ResponseWriter, req *http.Request, status int, content FileContent, body io.Reader) error {
	if body == nil {
		return errors.New("web: stream callback has no reader")
	}
	if err := req.Context().Err(); err != nil {
		return err
	}
	if err := setFileHeaders(w, content); err != nil {
		return err
	}
	if content.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(content.Size, 10))
	}
	w.WriteHeader(status)
	if req.Method == http.MethodHead {
		return nil
	}
	reader := responseContextReader{ctx: req.Context(), reader: body}
	if content.Size >= 0 {
		_, err := io.CopyN(w, reader, content.Size)
		return err
	}
	_, err := io.Copy(w, reader)
	return err
}

func (r *Router) sendCallbackStream(w http.ResponseWriter, req *http.Request, status int, out StreamResponse) error {
	if out.Stream == nil {
		return errors.New("web: StreamResponse has no producer")
	}
	sent := false
	var sendErr error
	err := out.Stream(req.Context(), func(content FileContent, body io.Reader) error {
		if sent {
			sendErr = errors.New("web: stream callback sent more than once")
			return sendErr
		}
		sent = true
		sendErr = sendStreamBody(w, req, status, content, body)
		return sendErr
	})
	if err == nil && !sent {
		return errors.New("web: stream callback sent no body")
	}
	return errors.Join(err, sendErr)
}

func (r *Router) responseFailure(w http.ResponseWriter, req *http.Request, err error) {
	r.logger.ErrorContext(req.Context(), "stream response failed", "error", err)
	if writer, ok := req.Context().Value(trackKey{}).(*trackWriter); ok && writer.wrote {
		panic(http.ErrAbortHandler)
	}
	r.encodeError(w, req, fmt.Errorf("write response: %w", err))
}
