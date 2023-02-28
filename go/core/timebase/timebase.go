package timebase

import (
	"sync/atomic"
	"time"

	"example.com/scion-time/go/base/timebase"
)

var (
	lclk atomic.Pointer[timebase.LocalClock]
)

func RegisterClock(c timebase.LocalClock) {
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
