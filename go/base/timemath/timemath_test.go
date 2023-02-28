package timemath_test

import (
	"math"
	"testing"
	"time"

	"example.com/scion-time/go/base/timemath"
)

func TestMedian(t *testing.T) {
	ds := []time.Duration{time.Duration(math.MaxInt64), time.Duration(math.MaxInt64 - 1)}
	m := ds[1]
	x := timemath.Median(ds)
	if x != m {
		t.Errorf("Median(%v) == %d; want %d", ds, x, m)
	}
}

func TestFaultTolerantMidpoint(t *testing.T) {
	ds := []time.Duration{
		time.Duration(math.MaxInt64),
		time.Duration(math.MaxInt64 - 1),
		time.Duration(math.MaxInt64 - 2),
		time.Duration(math.MaxInt64 - 3),
	}
	m := ds[2]
	x := timemath.FaultTolerantMidpoint(ds)
	if x != m {
		t.Errorf("FaultTolerantMidpoint(%v) == %d; want %d", ds, x, m)
	}
}
