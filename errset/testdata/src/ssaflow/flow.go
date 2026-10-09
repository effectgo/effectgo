// Package ssaflow checks the flow-sensitive error sets of the SSA engine
// (-errset.engine=ssa). The expectations here are the SSA engine's: the AST
// engine is flow-insensitive and gives the union for each of these functions.
package ssaflow

import "errors"

var (
	ErrA = errors.New("a")
	ErrB = errors.New("b")
)

func getA() (int, error) { return 1, ErrA } // want getA:"." "errors of getA: ssaflow.ErrA"
func getB() (int, error) { return 2, ErrB } // want getB:"." "errors of getB: ssaflow.ErrB"

// The second call overwrites err: only getB's error can reach the return.
func reuse() error { // want reuse:"." "errors of reuse: ssaflow.ErrB"
	_, err := getA()
	if err != nil {
		_ = err
	}
	_, err = getB()
	return err
}

// The nil assignment is the value that reaches the return.
func cleared() error { // want cleared:"." "errors of cleared: \\(none\\)"
	err := error(ErrA)
	_ = err
	err = nil
	return err
}

// Both branches reach the join: the phi carries both errors.
func either(n int) error { // want either:"." "errors of either: ssaflow.ErrA | ssaflow.ErrB"
	var err error
	if n > 0 {
		_, err = getA()
	} else {
		_, err = getB()
	}
	return err
}
