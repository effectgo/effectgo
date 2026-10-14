// Package enums is a golden case: a string enum and an int enum whose constants
// are declared out of order (the literals keep the source order), and a pointer
// to an enum.
package enums

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

// Status is a string enum.
type Status string

const (
	StatusPending  Status = "pending"
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// Valid reports whether s is one of the constants.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive, StatusArchived:
		return true
	}
	return false
}

// Priority is an int enum; its constants are not in numeric order.
type Priority int

const (
	PriorityHigh Priority = 3
	PriorityLow  Priority = 1
	PriorityMid  Priority = 2
)

// Valid reports whether p is one of the constants.
func (p Priority) Valid() bool {
	switch p {
	case PriorityHigh, PriorityLow, PriorityMid:
		return true
	}
	return false
}

// EnumIn is the request body of POST /api/enums.
type EnumIn struct {
	Status   Status   `json:"status"`
	Priority Priority `json:"priority"`
	Prev     *Status  `json:"prev,omitempty"`
}

// EnumOut is the response of POST /api/enums.
type EnumOut struct {
	Status   Status   `json:"status"`
	Priority Priority `json:"priority"`
	Prev     *Status  `json:"prev,omitempty"`
}

// Enums echoes the input.
func Enums(_ context.Context, in EnumIn) (EnumOut, error) {
	return EnumOut(in), nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Post(r, "/api/enums", Enums)
}
