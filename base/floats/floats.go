package floats

import (
	"slices"
)

func midpoint(x, y float64) float64 {
	return x + (y-x)/2.0
}

func Median(fs []float64) float64 {
	n := len(fs)
	if n == 0 {
		panic("unexpected number of values")
	}
	slices.Sort(fs)
	i := n / 2
	if n%2 != 0 {
		return fs[i]
	}
	return midpoint(fs[i-1], fs[i])
}

func FaultTolerantMidpoint(fs []float64) float64 {
	n := len(fs)
	if n == 0 {
		panic("unexpected number of values")
	}
	slices.Sort(fs)
	f := (n - 1) / 3
	return midpoint(fs[f], fs[n-1-f])
}
