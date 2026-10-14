package scope

import (
	"errors"
	"reflect"
	"testing"
)

type rec struct {
	name string
	log  *[]string
	err  error
}

func (r rec) Close() error {
	*r.log = append(*r.log, r.name)
	return r.err
}

func TestCloseReverseOrderAndJoin(t *testing.T) {
	var log []string
	errA := errors.New("a failed")
	errC := errors.New("c failed")
	var s Scope
	s.Add(rec{name: "a", log: &log, err: errA})
	s.Add(nil)
	s.Add(rec{name: "b", log: &log})
	s.Add(rec{name: "c", log: &log, err: errC})
	err := s.Close()
	if !reflect.DeepEqual(log, []string{"c", "b", "a"}) {
		t.Errorf("close order = %v", log)
	}
	if !errors.Is(err, errA) || !errors.Is(err, errC) {
		t.Errorf("joined error = %v", err)
	}
	log = nil
	if err := s.Close(); err != nil || len(log) != 0 {
		t.Errorf("second Close: err %v, closed again %v", err, log)
	}
}

func TestAddAfterClose(t *testing.T) {
	var log []string
	errLate := errors.New("late")
	var s Scope
	if err := s.Close(); err != nil {
		t.Fatalf("empty Close: %v", err)
	}
	s.Add(rec{name: "late", log: &log, err: errLate})
	if !reflect.DeepEqual(log, []string{"late"}) {
		t.Errorf("late closer not closed at once: %v", log)
	}
	if err := s.Close(); !errors.Is(err, errLate) {
		t.Errorf("late error not returned by next Close: %v", err)
	}
}

func TestZeroValueIsUsable(t *testing.T) {
	var s Scope
	var log []string
	s.Add(rec{name: "x", log: &log})
	if err := s.Close(); err != nil || len(log) != 1 {
		t.Errorf("zero value: %v %v", err, log)
	}
}
