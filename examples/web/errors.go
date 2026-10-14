package main

import (
	"net/http"
)

// ContactNotFound is returned when no contact has the requested id.
// Wire format: 404 {"_tag":"ContactNotFound","detail":"..."}.
type ContactNotFound struct {
	Detail string `json:"detail"`
}

func (e ContactNotFound) Error() string { return e.Detail }
func (ContactNotFound) StatusCode() int { return http.StatusNotFound }

// ValidationError is returned when a request is well formed JSON but its
// content is not acceptable. Wire format: 400 {"_tag":"ValidationError","detail":"..."}.
type ValidationError struct {
	Detail string `json:"detail"`
}

func (e ValidationError) Error() string { return e.Detail }
func (ValidationError) StatusCode() int { return http.StatusBadRequest }
