package client

import (
	"cmp"
	"slices"
)

func median(fs []float64) float64 {
	n := len(fs)
	if n == 0 {
		panic("unexpected number of values")
	}
	slices.SortFunc(fs, func(a, b float64) int {
		return cmp.Compare(a, b)
	})
	i := n / 2
	if n%2 != 0 {
		return fs[i]
	}
	return fs[i-1] + (fs[i]-fs[i-1])/2
}
