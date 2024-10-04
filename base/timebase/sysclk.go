package timebase

import (
	"time"
)

type SystemClock interface {
	Epoch() uint64
	Now() time.Time
	Drift(duration time.Duration) time.Duration
	Step(offset time.Duration)
	Adjust(offset, duration time.Duration, frequency float64)
	Sleep(duration time.Duration)
}
