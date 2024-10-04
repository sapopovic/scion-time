package timemath

import (
	"math"
	"slices"
	"time"
)

func Duration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

func Sgn(d time.Duration) int {
	switch {
	case d < 0:
		return -1
	case d > 0:
		return 1
	default:
		return 0
	}
}

func Inv(d time.Duration) time.Duration {
	switch {
	case d == math.MinInt64:
		return math.MaxInt64
	default:
		return -d
	}
}

func Midpoint(x, y time.Duration) time.Duration {
	return x + (y-x)/2.0
}

func Median(ds []time.Duration) time.Duration {
	n := len(ds)
	if n == 0 {
		panic("unexpected number of values")
	}
	slices.Sort(ds)
	i := n / 2
	if n%2 != 0 {
		return ds[i]
	}
	return Midpoint(ds[i-1], ds[i])
}

func FaultTolerantMidpoint(ds []time.Duration) time.Duration {
	n := len(ds)
	if n == 0 {
		panic("unexpected number of values")
	}
	slices.Sort(ds)
	f := (n - 1) / 3
	return Midpoint(ds[f], ds[n-1-f])
}
