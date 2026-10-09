package chain

import "time"

//effectgo:pure
func Normalize() string { // want Normalize:`map\[time:\[\{chain chain Normalize\} \{chain chain stamp\} \{time time Now\}\]\]` `Normalize is declared //effectgo:pure but has effect time via Normalize → stamp → time\.Now`
	return stamp()
}

func stamp() string { // want stamp:`map\[time:\[\{chain chain stamp\} \{time time Now\}\]\]`
	return time.Now().String()
}

// Deep's violation is the consequence of Normalize's: it is not reported
// (see TestSuppressed and -effects.all).
//
//effectgo:pure
func Deep() string { // want Deep:`map\[time:\[\{chain chain Deep\} \{chain chain Normalize\} \{chain chain stamp\} \{time time Now\}\]\]`
	return Normalize()
}

// Shortest reaches time.Now directly and through stamp; the direct witness
// is shorter and must be the one reported.
//
//effectgo:pure
func Shortest() string { // want Shortest:`map\[time:\[\{chain chain Shortest\} \{time time Now\}\]\]` `Shortest is declared //effectgo:pure but has effect time via Shortest → time\.Now$`
	_ = stamp()
	return time.Now().String()
}

//effectgo:pure
func Sum(a, b int) int { return a + b } // want Sum:`map\[\]`
