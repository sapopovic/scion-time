package core

import (
	"math"
	"sort"
	"time"
)

func Abs(d time.Duration) time.Duration {
	if d == math.MinInt64 {
		panic("unexpected duration value (math.MinInt64)")
	}
	if d < 0 {
		d = -d
	}
	return d
}

func Sign(d time.Duration) int {
	if d < 0 {
		return -1
	}
	if d > 0 {
		return 1
	}
	return 0
}

func Median(ds []time.Duration) time.Duration {
	n := len(ds)
	if n == 0 {
		panic("unexpected number of duration values")
	}
	sort.Slice(ds, func(i, j int) bool {
		return ds[i] < ds[j]
	})
	var m time.Duration
	i := n / 2
	if n%2 != 0 {
		m = ds[i]
	} else {
		m = (ds[i] + ds[i-1]) / 2
	}
	return m
}

func FaultTolerantMidpoint(ds []time.Duration) time.Duration {
	n := len(ds)
	if n == 0 {
		panic("unexpected number of duration values")
	}
	sort.Slice(ds, func(i, j int) bool {
		return ds[i] < ds[j]
	})
	var m time.Duration
	f := (n - 1) / 3
	m = (ds[f] + ds[n-1-f]) / 2
	return m
}
