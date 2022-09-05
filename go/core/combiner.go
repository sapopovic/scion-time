package core;

import (
	"time"

	"example.com/scion-time/go/core/timemath"
)

func Combine(lo, mid, hi time.Duration, trust float64) (offset time.Duration, weight float64) {
	offset = mid
	weight = 0.001 + trust * 2.0 / timemath.Seconds(hi -lo)
	if weight < 1.0 {
		weight = 1.0
	}
	return
}
