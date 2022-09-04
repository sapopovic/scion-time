package core;

import (
	"time"

	"example.com/scion-time/go/core/timemath"
)

func try(mx0, my0, l, m, h, trust, x float64) (mx1, my1 float64) {
	var y float64 = 0.001
	if x < m {
		y += trust * 2.0 * (x - l) / ((h - l) * (m - l))
	} else if x == m {
		y += trust * 2.0 / (h - l)
	} else {
		y += trust * 2.0 * (h - x) / ((h - l) * (h - m))
	}
	if y > my0 {
		mx1 = x
		my1 = y
	} else {
		mx1 = mx0
		my1 = my0
	}
	return
}

func Combine(lo, mid, hi time.Duration, trust float64) (offset time.Duration, weight float64) {
	var mx, my float64 = 0, 1
	l := timemath.Seconds(lo)
	m := timemath.Seconds(mid)
	h := timemath.Seconds(hi)
	mx, my = try(mx, my, l, m, h, trust, l)
	mx, my = try(mx, my, l, m, h, trust, m)
	mx, my = try(mx, my, l, m, h, trust, h)
	offset = timemath.Duration(mx)
	weight = my
	return
}
