package shm

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
)

const ReferenceClockType = "ntpshm"

var errNoSample = errors.New("SHM sample temporarily unavailable")

func MeasureClockOffset(ctx context.Context, log *zap.Logger, unit int) (time.Duration, error) {
	deadline, deadlineIsSet := ctx.Deadline()
	const maxNumRetries = 8
	numRetries := 0
	for {
		if !shmInitialized {
			err := initSHM(log, unit)
			if err != nil {
				if numRetries != maxNumRetries && deadlineIsSet && time.Now().Before(deadline) {
					time.Sleep(0)
					numRetries++
					continue
				}
				return 0, err
			}
		}

		tTimeMode := *shmTimeMode
		tTimeCount := *shmTimeCount
		tTimeValid := *shmTimeValid
		tTimeClockTimeStampSec := *shmTimeClockTimeStampSec
		tTimeClockTimeStampUSec := *shmTimeClockTimeStampUSec
		tTimeReceiveTimeStampSec := *shmTimeReceiveTimeStampSec
		tTimeReceiveTimeStampUSec := *shmTimeReceiveTimeStampUSec
		tTimeLeap := *shmTimeLeap
		tTimeClockTimeStampNSec := *shmTimeClockTimeStampNSec
		tTimeReceiveTimeStampNSec := *shmTimeReceiveTimeStampNSec

		// SHM client logic based on analogous code in chrony

		if (tTimeMode == 1 && tTimeCount != *shmTimeCount) ||
			!(tTimeMode == 0 || tTimeMode == 1) || tTimeValid == 0 {
			log.Error("SHM sample temporarily unavailable",
				zap.Int32("mode", tTimeMode), zap.Int32("count", tTimeCount), zap.Int32("valid", tTimeValid))
			if numRetries != maxNumRetries && deadlineIsSet && time.Now().Before(deadline) {
				time.Sleep(0)
				numRetries++
				continue
			}
			return 0, errNoSample
		}

		*shmTimeValid = 0

		receiveTimeSeconds := tTimeReceiveTimeStampSec
		clockTimeSeconds := tTimeClockTimeStampSec

		var receiveTimeNanoseconds, clockTimeNanoseconds int64
		if tTimeClockTimeStampNSec/1000 == uint32(tTimeClockTimeStampUSec) &&
			tTimeReceiveTimeStampNSec/1000 == uint32(tTimeReceiveTimeStampUSec) {
			receiveTimeNanoseconds = int64(tTimeReceiveTimeStampNSec)
			clockTimeNanoseconds = int64(tTimeClockTimeStampNSec)
		} else {
			receiveTimeNanoseconds = 1000 * int64(tTimeReceiveTimeStampUSec)
			clockTimeNanoseconds = 1000 * int64(tTimeClockTimeStampUSec)
		}

		receiveTime := time.Unix(receiveTimeSeconds, receiveTimeNanoseconds).UTC()
		clockTime := time.Unix(clockTimeSeconds, clockTimeNanoseconds).UTC()

		log.Debug("SHM clock sample",
			zap.Time("receiveTime", receiveTime),
			zap.Time("clockTime", clockTime),
			zap.Int32("leap", tTimeLeap),
		)

		offset := clockTime.Sub(receiveTime)

		log.Debug("SHM clock offset", zap.Duration("offset", offset))

		return offset, nil
	}
}
