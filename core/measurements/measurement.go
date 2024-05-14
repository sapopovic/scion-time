package measurements

import (
	"cmp"
	"slices"
	"time"
)

type Measurement struct {
	Timestamp time.Time
	Offset    time.Duration
	Error     error
}

func midpoint(x, y Measurement) Measurement {
	var m Measurement
	m.Offset = x.Offset + (y.Offset-x.Offset)/2
	if !x.Timestamp.After(y.Timestamp) {
		m.Timestamp = x.Timestamp.Add(y.Timestamp.Sub(x.Timestamp) / 2)
	} else {
		m.Timestamp = y.Timestamp.Add(x.Timestamp.Sub(y.Timestamp) / 2)
	}
	return m
}

func Median(ms []Measurement) Measurement {
	n := len(ms)
	if n == 0 {
		panic("unexpected number of values")
	}
	slices.SortFunc(ms, func(a, b Measurement) int {
		return cmp.Compare(a.Offset, b.Offset)
	})
	i := n / 2
	if n%2 != 0 {
		return Measurement{
			Timestamp: ms[i].Timestamp,
			Offset:    ms[i].Offset,
		}
	}
	return midpoint(ms[i-1], ms[i])
}

func FaultTolerantMidpoint(ms []Measurement) Measurement {
	n := len(ms)
	if n == 0 {
		panic("unexpected number of values")
	}
	slices.SortFunc(ms, func(a, b Measurement) int {
		return cmp.Compare(a.Offset, b.Offset)
	})
	f := (n - 1) / 3
	return midpoint(ms[f], ms[n-1-f])
}
