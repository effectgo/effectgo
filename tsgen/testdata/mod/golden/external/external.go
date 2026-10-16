// Package external exercises pre-handler middleware envelopes without wire tags.
package external

import (
	"context"
	"net/http"

	"github.com/faintghost/effectgo/rt/web"
)

type AuthenticationError struct {
	Detail string `json:"detail"`
}

func (AuthenticationError) Error() string   { return "authentication required" }
func (AuthenticationError) StatusCode() int { return http.StatusUnauthorized }

type DisabledCode string

const Disabled DisabledCode = "MODULE_DISABLED"

func (c DisabledCode) Valid() bool { return c == Disabled }

type MaintenanceCode string

const Maintenance MaintenanceCode = "MODULE_MAINTENANCE"

func (c MaintenanceCode) Valid() bool { return c == Maintenance }

type DisabledDetail struct {
	Error string       `json:"error"`
	Code  DisabledCode `json:"code"`
}
type MaintenanceDetail struct {
	Error string          `json:"error"`
	Code  MaintenanceCode `json:"code"`
}
type ModuleDisabled struct {
	Detail DisabledDetail `json:"detail"`
}
type ModuleMaintenance struct {
	Detail MaintenanceDetail `json:"detail"`
}

func (ModuleDisabled) Error() string      { return "module disabled" }
func (ModuleDisabled) StatusCode() int    { return http.StatusServiceUnavailable }
func (ModuleMaintenance) Error() string   { return "module maintenance" }
func (ModuleMaintenance) StatusCode() int { return http.StatusServiceUnavailable }

type Result struct {
	Name      string `json:"name"`
	RemoteSeq int64  `json:"remote_seq"`
}

type Input struct {
    Mode *string `query:"mode" json:"mode,omitempty"`
    Seq  *int64  `query:"seq" json:"seq,omitempty"`
}

func Read(_ context.Context, in Input) (Result, error) {
	if in.Mode != nil && *in.Mode == "auth" {
		return Result{}, AuthenticationError{Detail: "login required"}
	}
	return Result{Name: "reference", RemoteSeq: 9007199254740993}, nil
}
func Register(r *web.Router) {
	web.Get(r, "/api/external", Read,
		web.ExternalError[AuthenticationError](),
		web.ExternalError[ModuleDisabled](),
		web.ExternalError[ModuleMaintenance]())
}
