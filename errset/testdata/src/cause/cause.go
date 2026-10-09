// Package cause calls context.Cause, whose error set is the shipped rule in
// stubs/stdlib.stubs: the two sentinels, or the opaque cause of WithCancelCause.
package cause

import "context"

// useCause returns the cause of a done context.
func useCause(ctx context.Context) error { // want useCause:"." "errors of useCause: context.Canceled \\| context.DeadlineExceeded \\| \\?"
	return context.Cause(ctx)
}
