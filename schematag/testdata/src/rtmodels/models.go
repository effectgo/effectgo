// Package rtmodels is a domain package. It does not import rt, so its types are
// checked only where a route or a decode brings them into rt (rtuse/cross).
package rtmodels

// Status is a sealed enum without a Valid method.
//
//effectgo:sealed
type Status string

// StatusOpen is a value of Status.
const StatusOpen Status = "open"

// Contact is a domain type with a bad schema tag and an enum field.
type Contact struct {
	Name   string `json:"name" schema:"trim,nonempty,bogus"`
	Status Status `json:"status"`
}
