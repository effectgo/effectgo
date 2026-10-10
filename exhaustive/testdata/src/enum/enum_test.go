package enum

// TestOnly is a value of Status declared in a test file. It is not a variant:
// the switch in Label still misses only Archived.
const TestOnly Status = 42
