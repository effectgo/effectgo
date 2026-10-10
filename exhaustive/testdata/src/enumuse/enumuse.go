package enumuse

import "enum"

// Across packages only the exported values are variants.
func Describe(s enum.Status) string {
	switch s { // want "missing cases in switch on Status: Archived, Done, Open"
	case enum.AnyStatus:
		return "any"
	}
	return ""
}

func Full(s enum.Status) string {
	switch s {
	case enum.AnyStatus, enum.Open, enum.Done, enum.Archived:
		return "all"
	}
	return ""
}

// Level's unexported value is not a variant here.
func Lvl(l enum.Level) string {
	switch l {
	case enum.Low, enum.High:
		return "known"
	}
	return ""
}
