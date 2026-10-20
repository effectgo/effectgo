package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type responseTestFile struct {
	*strings.Reader
	closed int
}

func (f *responseTestFile) Close() error { f.closed++; return nil }

func TestTypedFileResponsePreservesRangeAndCloses(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			body := &responseTestFile{Reader: strings.NewReader("0123456789")}
			router := New()
			Get(router, "/file", func(context.Context, NoInput) (FileResponse, error) {
				return FileResponse{Body: body, Filename: "stock.csv", ContentType: "text/csv", Size: 10, ModTime: time.Unix(1700000000, 0)}, nil
			})
			req := httptest.NewRequest(method, "/file", nil)
			req.Header.Set("Range", "bytes=2-5")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusPartialContent || rec.Header().Get("Content-Range") != "bytes 2-5/10" || rec.Header().Get("Content-Type") != "text/csv" || body.closed != 1 {
				t.Fatalf("range contract: %d %v closed=%d", rec.Code, rec.Header(), body.closed)
			}
			want := "2345"
			if method == http.MethodHead {
				want = ""
			}
			if rec.Body.String() != want {
				t.Fatalf("body=%q want=%q", rec.Body.String(), want)
			}
		})
	}
}

func TestFileResponseConditionalAndBodylessHeaders(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent, http.StatusResetContent} {
		body := &responseTestFile{Reader: strings.NewReader("file")}
		router := New()
		Get(router, "/file", func(context.Context, NoInput) (FileResponse, error) {
			return FileResponse{Body: body, Status: status, Size: 4, ModTime: time.Unix(1700000000, 0), Header: http.Header{"Cache-Control": {"private, no-store"}, "Set-Cookie": {"session=; Max-Age=0; Path=/"}}}, nil
		})
		req := httptest.NewRequest(http.MethodGet, "/file", nil)
		if status == http.StatusOK {
			req.Header.Set("If-Modified-Since", time.Unix(1700000000, 0).UTC().Format(http.TimeFormat))
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		want := status
		if status == http.StatusOK {
			want = http.StatusNotModified
		}
		if rec.Code != want || rec.Body.Len() != 0 || rec.Header().Get("Cache-Control") != "private, no-store" || rec.Header().Get("Set-Cookie") == "" || body.closed != 1 {
			t.Fatalf("bodyless contract status=%d got=%d body=%q headers=%v closed=%d", status, rec.Code, rec.Body.String(), rec.Header(), body.closed)
		}
	}
}

func TestFileResponsePreservesExplicitInlineDisposition(t *testing.T) {
	router := New()
	Get(router, "/label", func(context.Context, NoInput) (FileResponse, error) {
		return FileResponse{Body: io.NopCloser(strings.NewReader("PDF")), Filename: "label.pdf", Size: 3, ContentType: "application/octet-stream", Header: http.Header{"Content-Disposition": {`inline; filename="label.pdf"`}, "Content-Type": {"application/pdf"}}}, nil
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/label", nil))
	if rec.Header().Get("Content-Disposition") != `inline; filename="label.pdf"` || rec.Header().Get("Content-Type") != "application/pdf" || rec.Body.String() != "PDF" {
		t.Fatalf("inline response changed: %v %q", rec.Header(), rec.Body.String())
	}
}

func TestTypedResponseSuccessStatusAndMetadata(t *testing.T) {
	type payload struct {
		ID int `json:"id"`
	}
	router := New()
	Post(router, "/created", func(context.Context, NoInput) (payload, error) { return payload{ID: 4}, nil }, SuccessStatus(http.StatusCreated))
	Get(router, "/proxy", func(context.Context, NoInput) (Response[payload], error) {
		return Response[payload]{Body: payload{ID: 9}, Status: http.StatusConflict, Header: http.Header{"Content-Type": {"application/problem+json"}, "X-Provider": {"test"}}}, nil
	})
	Get(router, "/redirect", func(context.Context, NoInput) (RedirectResponse, error) {
		return RedirectResponse{Location: "/settings?connected=yes"}, nil
	})
	for _, tc := range []struct {
		method, path string
		status       int
		body, media  string
	}{
		{http.MethodPost, "/created", 201, `{"id":4}`, "application/json"},
		{http.MethodGet, "/proxy", 409, `{"id":9}`, "application/problem+json"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.status || rec.Body.String() != tc.body || rec.Header().Get("Content-Type") != tc.media {
			t.Fatalf("%s: %d %s %v", tc.path, rec.Code, rec.Body.String(), rec.Header())
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/redirect", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/settings?connected=yes" {
		t.Fatalf("redirect: %d %v", rec.Code, rec.Header())
	}
}

func TestProxyResponseSelectsTypedBodyAndPreservesStatus(t *testing.T) {
	type success struct {
		ID int `json:"id"`
	}
	type failure struct {
		Code    string            `json:"code"`
		Details map[string]string `json:"details"`
	}
	for _, tc := range []struct {
		name          string
		successStatus int
		status        int
	}{
		{name: "default success"},
		{name: "created success", successStatus: http.StatusCreated},
		{name: "rate limit", status: http.StatusTooManyRequests},
		{name: "partial failure", status: http.StatusPartialContent},
		{name: "storage failure", status: http.StatusInsufficientStorage},
		{name: "provider extension", status: 777},
		{name: "highest status", status: 999},
		{name: "200 failure with 201 success", successStatus: http.StatusCreated, status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := "application/json"
			if tc.status != 0 {
				media = "application/problem+json"
			}
			router := New()
			var opts []RouteOption
			if tc.successStatus != 0 {
				opts = append(opts, SuccessStatus(tc.successStatus))
			}
			Get(router, "/proxy", func(context.Context, NoInput) (ProxyResponse[success, failure], error) {
				return ProxyResponse[success, failure]{
					Body:    success{ID: 9},
					Failure: failure{Code: "provider_busy", Details: map[string]string{"provider": "arc"}},
					Status:  tc.status,
					Header:  http.Header{"Content-Type": {media}, "Retry-After": {"30"}, "Set-Cookie": {"a=1", "b=2"}},
				}, nil
			}, opts...)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))
			wantStatus, wantBody := tc.status, `{"code":"provider_busy","details":{"provider":"arc"}}`
			if tc.status == 0 {
				wantStatus, wantBody = tc.successStatus, `{"id":9}`
				if wantStatus == 0 {
					wantStatus = http.StatusOK
				}
			}
			if rec.Code != wantStatus || rec.Body.String() != wantBody {
				t.Fatalf("status=%d body=%s; want status=%d body=%s", rec.Code, rec.Body.String(), wantStatus, wantBody)
			}
			cookies := rec.Header().Values("Set-Cookie")
			if rec.Header().Get("Content-Type") != media || rec.Header().Get("Retry-After") != "30" || len(cookies) != 2 || cookies[0] != "a=1" || cookies[1] != "b=2" {
				t.Fatalf("provider headers changed: %v", rec.Header())
			}
		})
	}
}

func TestProxyResponseRejectsStatusBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		successStatus int
		status        int
	}{
		{http.StatusOK, -1},
		{http.StatusOK, 99},
		{http.StatusOK, 1000},
		{http.StatusOK, http.StatusOK},
		{http.StatusCreated, http.StatusCreated},
	} {
		t.Run(fmt.Sprintf("%d/default%d", tc.status, tc.successStatus), func(t *testing.T) {
			rec := httptest.NewRecorder()
			encoded := false
			router := New(WithErrorEncoder(func(w http.ResponseWriter, req *http.Request, err error) {
				encoded = true
				writer := req.Context().Value(trackKey{}).(*trackWriter)
				if writer.wrote || rec.Body.Len() != 0 || rec.Header().Get("X-Provider") != "" {
					t.Fatal("invalid proxy response committed before error encoding")
				}
				if !strings.Contains(err.Error(), "invalid proxy response status") {
					t.Fatalf("status validation did not precede body encoding: %v", err)
				}
				w.WriteHeader(http.StatusInternalServerError)
			}))
			Get(router, "/proxy", func(context.Context, NoInput) (ProxyResponse[struct{}, chan int], error) {
				return ProxyResponse[struct{}, chan int]{Status: tc.status, Failure: make(chan int), Header: http.Header{"X-Provider": {"must not leak"}}}, nil
			}, SuccessStatus(tc.successStatus))
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))
			if !encoded || rec.Code != http.StatusInternalServerError {
				t.Fatalf("invalid status accepted: encoded=%v status=%d body=%s", encoded, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestProxyResponseOmitsBodyForBodylessStatus(t *testing.T) {
	for _, status := range []int{100, 103, 199, http.StatusNoContent, http.StatusResetContent, http.StatusNotModified} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			router := New()
			Get(router, "/proxy", func(context.Context, NoInput) (ProxyResponse[struct{}, chan int], error) {
				// A bodyless status must not attempt to serialize this body.
				return ProxyResponse[struct{}, chan int]{Status: status, Failure: make(chan int), Header: http.Header{"X-Provider": {"arc"}}}, nil
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy", nil))
			if rec.Code != status || rec.Body.Len() != 0 || rec.Header().Get("X-Provider") != "arc" {
				t.Fatalf("bodyless proxy response: status=%d body=%q headers=%v", rec.Code, rec.Body.String(), rec.Header())
			}
		})
	}
}

func TestProxyResponseHeadPreservesMetadataWithoutBody(t *testing.T) {
	for _, status := range []int{0, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			router := New()
			Get(router, "/proxy", func(context.Context, NoInput) (ProxyResponse[string, string], error) {
				return ProxyResponse[string, string]{Body: "success", Failure: "failure", Status: status, Header: http.Header{"X-Provider": {"arc"}}}, nil
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/proxy", nil))
			wantStatus := status
			if wantStatus == 0 {
				wantStatus = http.StatusOK
			}
			if rec.Code != wantStatus || rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("X-Provider") != "arc" {
				t.Fatalf("HEAD proxy response: status=%d body=%q headers=%v", rec.Code, rec.Body.String(), rec.Header())
			}
		})
	}
}

func TestProxyResponseRejectsAlternativeSuccessStatuses(t *testing.T) {
	for _, option := range []RouteOption{SuccessStatuses(200, 201), SuccessStatusRange(200, 202)} {
		func() {
			defer func() {
				if got := recover(); got != "web: ProxyResponse requires a single success status" {
					t.Fatalf("alternative success statuses accepted: %v", got)
				}
			}()
			Get(New(), "/proxy", func(context.Context, NoInput) (ProxyResponse[struct{}, struct{}], error) {
				return ProxyResponse[struct{}, struct{}]{}, nil
			}, option)
		}()
	}
}

func TestCallbackStreamKeepsProviderLifetimeAndReportsTruncation(t *testing.T) {
	t.Run("callback lifetime", func(t *testing.T) {
		router := New()
		providerActive := false
		Get(router, "/attachment", func(context.Context, NoInput) (StreamResponse, error) {
			return StreamResponse{Stream: func(_ context.Context, send func(FileContent, io.Reader) error) error {
				providerActive = true
				defer func() { providerActive = false }()
				return send(FileContent{Size: 3, ContentType: "application/pdf", Header: http.Header{"Cache-Control": {"private, no-store"}}}, strings.NewReader("PDF"))
			}}, nil
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/attachment", nil))
		if providerActive || rec.Body.String() != "PDF" || rec.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("callback result: active=%v body=%q headers=%v", providerActive, rec.Body.String(), rec.Header())
		}
	})
	t.Run("truncation aborts", func(t *testing.T) {
		router := New()
		Get(router, "/attachment", func(context.Context, NoInput) (StreamResponse, error) {
			return StreamResponse{Stream: func(_ context.Context, send func(FileContent, io.Reader) error) error {
				return send(FileContent{Size: 8}, strings.NewReader("short"))
			}}, nil
		})
		rec := httptest.NewRecorder()
		defer func() {
			if got := recover(); got != http.ErrAbortHandler {
				t.Fatalf("truncated response must abort, got %v", got)
			}
			if rec.Body.String() != "short" {
				t.Fatalf("JSON appended after stream: %q", rec.Body.String())
			}
		}()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/attachment", nil))
	})
}

func TestFileResponseClosesOnCancellation(t *testing.T) {
	body := &responseTestFile{Reader: strings.NewReader("secret")}
	router := New()
	Get(router, "/file", func(context.Context, NoInput) (FileResponse, error) { return FileResponse{Body: body, Size: 6}, nil })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/file", nil).WithContext(ctx)
	func() {
		defer func() {
			if p := recover(); p != nil && p != http.ErrAbortHandler {
				panic(p)
			}
		}()
		router.ServeHTTP(rec, req)
	}()
	if body.closed != 1 || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("canceled response leaked bytes or body: closed=%d body=%q", body.closed, rec.Body.String())
	}
}

func TestResponseRejectsInvalidOutputContract(t *testing.T) {
	for name, register := range map[string]func(){
		"204 JSON": func() {
			Get(New(), "/x", func(context.Context, NoInput) (struct{}, error) { return struct{}{}, nil }, SuccessStatus(204))
		},
		"JSON redirect": func() {
			Get(New(), "/x", func(context.Context, NoInput) (struct{}, error) { return struct{}{}, nil }, SuccessStatus(302))
		},
		"duplicate": func() {
			Get(New(), "/x", func(context.Context, NoInput) (NoContent, error) { return NoContent{}, nil }, SuccessStatus(204), SuccessStatus(204))
		},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid response accepted")
				}
			}()
			register()
		})
	}
}
