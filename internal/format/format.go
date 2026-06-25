// Package format mirrors the client's display helpers (data/hybreed.ts `fmt`) so
// the API can return both raw values and the strings the UI renders.
package format

import (
	"fmt"
	"math"
)

// Pace renders seconds-per-km as "m:ss".
func Pace(secPerKm int) string {
	if secPerKm <= 0 {
		return "0:00"
	}
	return fmt.Sprintf("%d:%02d", secPerKm/60, secPerKm%60)
}

// Clock renders a duration in seconds as "h:mm:ss" (or "m:ss" under an hour).
func Clock(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// Km converts metres to kilometres rounded to one decimal place.
func Km(meters int) float64 {
	return math.Round(float64(meters)/100) / 10
}
