package shm

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

const ReferenceClockType = "ntpshm"

type ReferenceClock struct {
	unit int
	shm  segment
}

var errNoSample = errors.New("SHM sample temporarily unavailable")

func NewReferenceClock(unit int) *ReferenceClock {
	return &ReferenceClock{unit: unit}
}

func (c *ReferenceClock) MeasureClockOffset(ctx context.Context, log *zap.Logger) (time.Duration, error) {
	deadline, deadlineIsSet := ctx.Deadline()
	const maxNumRetries = 8
	numRetries := 0
	for {
		if !c.shm.initialized {
			err := initSegment(&c.shm, c.unit)
			if err != nil {
				if numRetries != maxNumRetries && deadlineIsSet && time.Now().Before(deadline) {
					time.Sleep(0)
					numRetries++
					continue
				}
				return 0, err
			}
		}

		t := *c.shm.time

		// SHM client logic based on analogous code in chrony

		if (t.mode == 1 && t.count != c.shm.time.count) ||
			!(t.mode == 0 || t.mode == 1) || t.valid == 0 {
			log.Error("SHM sample temporarily unavailable",
				zap.Int32("mode", t.mode), zap.Int32("count", t.count), zap.Int32("valid", t.valid))
			if numRetries != maxNumRetries && deadlineIsSet && time.Now().Before(deadline) {
				time.Sleep(0)
				numRetries++
				continue
			}
			return 0, errNoSample
		}

		c.shm.time.valid = 0

		receiveTimeSeconds := t.receiveTimeStampSec
		clockTimeSeconds := t.clockTimeStampSec

		var receiveTimeNanoseconds, clockTimeNanoseconds int64
		if t.clockTimeStampNSec/1000 == uint32(t.clockTimeStampUSec) &&
			t.receiveTimeStampNSec/1000 == uint32(t.receiveTimeStampUSec) {
			receiveTimeNanoseconds = int64(t.receiveTimeStampNSec)
			clockTimeNanoseconds = int64(t.clockTimeStampNSec)
		} else {
			receiveTimeNanoseconds = 1000 * int64(t.receiveTimeStampUSec)
			clockTimeNanoseconds = 1000 * int64(t.clockTimeStampUSec)
		}

		receiveTime := time.Unix(receiveTimeSeconds, receiveTimeNanoseconds).UTC()
		clockTime := time.Unix(clockTimeSeconds, clockTimeNanoseconds).UTC()
		offset := clockTime.Sub(receiveTime)

		log.Debug("SHM clock sample",
			zap.Time("receiveTime", receiveTime),
			zap.Time("clockTime", clockTime),
			zap.Duration("offset", offset),
		)

		return offset, nil
	}
}
