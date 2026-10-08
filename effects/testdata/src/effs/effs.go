package effs

import (
	"os"
	"time"
)

var counter int

//effectgo:pure
func Spawn() { // want Spawn:`map\[spawn:` `Spawn is declared //effectgo:pure but has effect spawn via Spawn → go statement`
	go func() {}()
}

//effectgo:pure
func Boom() { // want Boom:`map\[panic:` `Boom is declared //effectgo:pure but has effect panic via Boom → panic`
	panic("boom")
}

//effectgo:pure
func Bump() { // want Bump:`map\[global:` `Bump is declared //effectgo:pure but has effect global via Bump → counter`
	counter++
}

//effectgo:pure
func Closure() func() { // want Closure:`map\[time:` `Closure is declared //effectgo:pure but has effect time via Closure → time\.Now`
	return func() {
		_ = time.Now()
	}
}

//effectgo:pure
func ClosureWrite() { // want ClosureWrite:`map\[global:` `ClosureWrite is declared //effectgo:pure but has effect global via ClosureWrite → counter`
	func() { counter = 2 }()
}

//effectgo:allow time
func Env() string { // want Env:`map\[env:` `Env is declared //effectgo:allow time but has effect env via Env → os\.Getenv`
	return os.Getenv("HOME")
}

//effectgo:pure
func Out() { // want Out:`map\[stdio:` `Out is declared //effectgo:pure but has effect stdio via Out → os\.Stdout`
	_ = os.Stdout
}
