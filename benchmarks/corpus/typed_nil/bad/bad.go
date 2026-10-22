package bad

type Problem struct{}

func (*Problem) Error() string { return "problem" }

func IsMissing() bool {
	var pointer *Problem
	var err error = pointer
	return err == nil
}
