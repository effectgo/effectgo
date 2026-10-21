// Package contractcase has one consumer-visible violation per static analyzer.
package contractcase

import (
	"context"
	"errors"
	"time"

	"github.com/faintghost/effectgo/rt/web"
)

//effectgo:pure
func Clock() time.Time { return time.Now() }

var ErrDomain = errors.New("domain failure")
var ErrOther = errors.New("other failure")

//effectgo:errors ErrOther
func Save() error { return ErrDomain }

//effectgo:mustuse
type Ticket struct{}

func Start() Ticket { return Ticket{} }
func Discard()      { Start() }

//effectgo:sealed
type Mode int

const (
	Open Mode = iota
	Closed
)

func Label(mode Mode) string {
	switch mode {
	case Open:
		return "open"
	}
	return ""
}

//effectgo:pure
var Misplaced = 1

type Input struct {
	Email int `json:"email" schema:"email"`
}

type Output struct {
	OK bool `json:"ok"`
}

func Submit(context.Context, Input) (Output, error) { return Output{OK: true}, nil }

func Register(r *web.Router) {
	web.Post(r, "/native", Submit)
}
