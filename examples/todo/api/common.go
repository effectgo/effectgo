// Package api registers the todo routes on one Fuego server: /v1 with
// idiomatic (T, error) handlers and /v2 with Result-returning handlers.
package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/effectgo/effectgo/examples/todo/domain"
	"github.com/effectgo/effectgo/examples/todo/httpres"
	"github.com/go-fuego/fuego"
)

// createReq is the POST /todos body (shared by v1 and v2).
type createReq struct {
	Title string `json:"title"`
}

// updateReq is the PUT /todos/{id} body (shared by v1 and v2). Nil fields
// are left unchanged.
type updateReq struct {
	Title *string `json:"title,omitempty"`
	Done  *bool   `json:"done,omitempty"`
}

// deleted is the response of the DELETE routes: an empty object. It is named
// because Fuego documents an unnamed struct{} response as a schema with an empty
// name, which OpenAPI validators reject (openapi_validate_test.go).
type deleted struct{}

// parseID reads the {id} path parameter. Any non-positive-integer value is a
// 400. (Fuego's own PathParamIntErr would return 422, so it is not used.)
func parseID(c interface{ PathParam(string) string }) (int, error) {
	raw := c.PathParam("id")
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, fuego.BadRequestError{Err: err, Detail: fmt.Sprintf("invalid todo id %q", raw)}
	}
	return id, nil
}

// NewServer builds one Fuego server with both route sets registered. The
// extra options (for example fuego.WithAddr) are applied after the defaults.
func NewServer(svc *domain.Service, extra ...fuego.ServerOption) *fuego.Server {
	// Fuego's default OpenAPI config writes doc/openapi.json into the working
	// directory when the spec is generated. Keep the rest of the defaults
	// (spec URL, Swagger UI) and turn off only the file write.
	oa := fuego.OpenAPIConfig{
		SpecURL:          "/swagger/openapi.json",
		SwaggerURL:       "/swagger",
		UIHandler:        fuego.DefaultOpenAPIHandler,
		DisableLocalSave: true,
		DisableMessages:  true,
	}
	opts := []fuego.ServerOption{
		fuego.WithoutLogger(),
		fuego.WithoutStartupMessages(),
		fuego.WithEngineOptions(
			fuego.WithOpenAPIConfig(oa),
			fuego.WithErrorHandler(errorHandler),
		),
	}
	s := fuego.NewServer(append(opts, extra...)...)
	registerV1(s, svc)
	registerV2(s, svc)
	registerV3(s, svc)
	return s
}

// errorHandler is the server-wide error mapping used by v3. It maps domain
// errors to HTTP errors through httpres.ToHTTPError and then calls Fuego's
// default handler. Errors that v1 and v2 already convert pass through
// ToHTTPError unchanged, so those routes behave as before.
func errorHandler(ctx context.Context, err error) error {
	return fuego.ErrorHandler(ctx, httpres.ToHTTPError(err))
}

// docOpts are the route options applied identically to v1 and v2 so the
// generated OpenAPI documents the same responses for both.
func docOpts(opId string, extra ...fuego.RouteOption) []fuego.RouteOption {
	opts := []fuego.RouteOption{
		fuego.OptionOperationID(opId),
		fuego.OptionAddError(http.StatusNotFound, "Todo not found", fuego.HTTPError{}),
	}
	return append(opts, extra...)
}
