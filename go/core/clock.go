package core

import (
	"fmt"
	"time"
)

type LocalClock interface {
	Now() time.Time
	MaxDrift(duration time.Duration) time.Duration
	Step(offset time.Duration)
	Adjust(offset, duration time.Duration) // TODO: add argument 'frequency float64'
	Sleep(duration time.Duration)
}

var errNoClockMeasurements = fmt.Errorf("failed to measure clock values")
