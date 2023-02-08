package timebase

import (
	"sync/atomic"
	"time"
)

type LocalClock interface {
	Epoch() uint64
	Now() time.Time
	MaxDrift(duration time.Duration) time.Duration
	Step(offset time.Duration)
	Adjust(offset, duration time.Duration, frequency float64)
	Sleep(duration time.Duration)
}

var (
	lclk atomic.Pointer[LocalClock]
)

func RegisterClock(c LocalClock) {
	if c == nil {
		panic("local clock must not be nil")
	}
	swapped := lclk.CompareAndSwap(nil, &c)
	if !swapped {
		panic("local clock already registered")
	}
}

func Now() time.Time {
	c := *lclk.Load()
	if c == nil {
		panic("no local clock registered")
	}
	return c.Now()
}

func Epoch() uint64 {
	c := *lclk.Load()
	if c == nil {
		panic("no local clock registered")
	}
	return c.Epoch()
}
