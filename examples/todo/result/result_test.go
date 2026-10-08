package result

import (
	"errors"
	"strconv"
	"testing"
)

var errBoom = errors.New("boom")

func TestOkErr(t *testing.T) {
	o := Ok(3)
	if !o.IsOk() {
		t.Fatal("Ok.IsOk = false")
	}
	if v, err := o.Get(); v != 3 || err != nil {
		t.Fatalf("Ok.Get = %v, %v", v, err)
	}
	e := Err[int](errBoom)
	if e.IsOk() {
		t.Fatal("Err.IsOk = true")
	}
	if v, err := e.Get(); v != 0 || !errors.Is(err, errBoom) {
		t.Fatalf("Err.Get = %v, %v", v, err)
	}
}

func TestErrNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Err(nil) did not panic")
		}
	}()
	Err[int](nil)
}

func TestFrom(t *testing.T) {
	if v, err := From(5, nil).Get(); v != 5 || err != nil {
		t.Fatalf("From(5,nil) = %v,%v", v, err)
	}
	if _, err := From(5, errBoom).Get(); !errors.Is(err, errBoom) {
		t.Fatalf("From(5,boom) err = %v", err)
	}
}

func TestUnwrap(t *testing.T) {
	if Ok("x").Unwrap() != "x" {
		t.Fatal("Unwrap on Ok")
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Unwrap on Err did not panic")
		}
	}()
	Err[string](errBoom).Unwrap()
}

func TestUnwrapOr(t *testing.T) {
	if Ok(1).UnwrapOr(9) != 1 {
		t.Fatal("UnwrapOr on Ok")
	}
	if Err[int](errBoom).UnwrapOr(9) != 9 {
		t.Fatal("UnwrapOr on Err")
	}
}

func TestMap(t *testing.T) {
	called := false
	f := func(i int) string { called = true; return strconv.Itoa(i * 2) }
	r := Map(Ok(4), f)
	if v, err := r.Get(); v != "8" || err != nil || !called {
		t.Fatalf("Map Ok = %q,%v called=%v", v, err, called)
	}
	called = false
	r = Map(Err[int](errBoom), f)
	if _, err := r.Get(); !errors.Is(err, errBoom) || called {
		t.Fatalf("Map Err: err=%v called=%v", err, called)
	}
}

func TestAndThen(t *testing.T) {
	half := func(i int) Result[int] {
		if i%2 != 0 {
			return Err[int](errors.New("odd"))
		}
		return Ok(i / 2)
	}
	if v, err := AndThen(Ok(8), half).Get(); v != 4 || err != nil {
		t.Fatalf("AndThen Ok->Ok = %v,%v", v, err)
	}
	if _, err := AndThen(Ok(3), half).Get(); err == nil || err.Error() != "odd" {
		t.Fatalf("AndThen Ok->Err = %v", err)
	}
	called := false
	r := AndThen(Err[int](errBoom), func(i int) Result[int] { called = true; return Ok(i) })
	if _, err := r.Get(); !errors.Is(err, errBoom) || called {
		t.Fatalf("AndThen Err short-circuit: err=%v called=%v", err, called)
	}
}

func TestMapErr(t *testing.T) {
	wrapped := errors.New("wrapped")
	r := MapErr(Err[int](errBoom), func(error) error { return wrapped })
	if _, err := r.Get(); err != wrapped {
		t.Fatalf("MapErr Err = %v", err)
	}
	r = MapErr(Ok(1), func(error) error { t.Fatal("called on Ok"); return nil })
	if v, _ := r.Get(); v != 1 {
		t.Fatal("MapErr Ok changed value")
	}
}
