package witness

import "time"

func Now() time.Time { return time.Now() } // want "witness of Now: time via time.Now" Now:"."

func Outer() time.Time { return Now() } // want "witness of Outer: time via Now → time.Now" Outer:"."
