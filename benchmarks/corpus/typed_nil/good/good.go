package good

type Problem struct{}

func (*Problem) Error() string { return "problem" }

func IsMissing() bool {
	var pointer *Problem
	return pointer == nil
}
