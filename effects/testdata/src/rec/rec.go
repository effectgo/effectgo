package rec

import "time"

//effectgo:pure
func Ping(n int) int { // want Ping:`map\[time:\[\{rec rec Ping\} \{rec rec Pong\} \{time time Sleep\}\]\]` `Ping is declared //effectgo:pure but has effect time via Ping → Pong → time\.Sleep`
	if n == 0 {
		return 0
	}
	return Pong(n - 1)
}

func Pong(n int) int { // want Pong:`map\[time:\[\{rec rec Pong\} \{time time Sleep\}\]\]`
	time.Sleep(0)
	return Ping(n - 1)
}

//effectgo:pure
func Even(n int) bool { // want Even:`map\[\]`
	if n == 0 {
		return true
	}
	return Odd(n - 1)
}

//effectgo:pure
func Odd(n int) bool { // want Odd:`map\[\]`
	if n == 0 {
		return false
	}
	return Even(n - 1)
}

//effectgo:pure
func SelfLoop(n int) int { // want SelfLoop:`map\[\]`
	if n == 0 {
		return 0
	}
	return SelfLoop(n - 1)
}
