package errset

import "golang.org/x/tools/go/ssa"

// narrowEdge narrows the environment e of the edge p -> b by the branch that
// ends p. Milestone M1 does not narrow: the environment stays as it is.
func (f *ssaFn) narrowEdge(p, b *ssa.BasicBlock, e map[ssa.Value]errSet) {}
