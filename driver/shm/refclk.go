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

		timeMode := *c.shm.timeMode
		timeCount := *c.shm.timeCount
		timeValid := *c.shm.timeValid
		timeClockTimeStampSec := *c.shm.timeClockTimeStampSec
		timeClockTimeStampUSec := *c.shm.timeClockTimeStampUSec
		timeReceiveTimeStampSec := *c.shm.timeReceiveTimeStampSec
		timeReceiveTimeStampUSec := *c.shm.timeReceiveTimeStampUSec
		timeLeap := *c.shm.timeLeap
		timeClockTimeStampNSec := *c.shm.timeClockTimeStampNSec
		timeReceiveTimeStampNSec := *c.shm.timeReceiveTimeStampNSec

		// SHM client logic based on analogous code in chrony

		if (timeMode == 1 && timeCount != *c.shm.timeCount) ||
			!(timeMode == 0 || timeMode == 1) || timeValid == 0 {
			log.Error("SHM sample temporarily unavailable",
				zap.Int32("mode", timeMode), zap.Int32("count", timeCount), zap.Int32("valid", timeValid))
			if numRetries != maxNumRetries && deadlineIsSet && time.Now().Before(deadline) {
				time.Sleep(0)
				numRetries++
				continue
			}
			return 0, errNoSample
		}

		*c.shm.timeValid = 0

		receiveTimeSeconds := timeReceiveTimeStampSec
		clockTimeSeconds := timeClockTimeStampSec

		var receiveTimeNanoseconds, clockTimeNanoseconds int64
		if timeClockTimeStampNSec/1000 == uint32(timeClockTimeStampUSec) &&
			timeReceiveTimeStampNSec/1000 == uint32(timeReceiveTimeStampUSec) {
			receiveTimeNanoseconds = int64(timeReceiveTimeStampNSec)
			clockTimeNanoseconds = int64(timeClockTimeStampNSec)
		} else {
			receiveTimeNanoseconds = 1000 * int64(timeReceiveTimeStampUSec)
			clockTimeNanoseconds = 1000 * int64(timeClockTimeStampUSec)
		}

		receiveTime := time.Unix(receiveTimeSeconds, receiveTimeNanoseconds).UTC()
		clockTime := time.Unix(clockTimeSeconds, clockTimeNanoseconds).UTC()

		log.Debug("SHM clock sample",
			zap.Time("receiveTime", receiveTime),
			zap.Time("clockTime", clockTime),
			zap.Int32("leap", timeLeap),
		)

		offset := clockTime.Sub(receiveTime)

		log.Debug("SHM clock offset", zap.Duration("offset", offset))

		return offset, nil
	}
}
