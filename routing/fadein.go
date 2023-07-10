package routing

import (
	"math"
	"time"
)

func fadeIn(now time.Time, duration time.Duration, exponent float64, detected time.Time) float64 {
	rel := now.Sub(detected)
	fadingIn := rel > 0 && rel < duration
	if !fadingIn {
		return 1
	}

	return math.Pow(float64(rel)/float64(duration), exponent)
}
