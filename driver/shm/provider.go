package shm

import (
	"time"

	"go.uber.org/zap"
)

func StoreClockSamples(log *zap.Logger, refTime, sysTime time.Time) error {
	if !shmInitialized {
		err := initSHM(log)
		if err != nil {
			return err
		}
	}

	*shmTimeMode = 0
	*shmTimeClockTimeStampSec = refTime.Unix()
	*shmTimeClockTimeStampUSec = int32(refTime.Nanosecond() / 1e3)
	*shmTimeReceiveTimeStampSec = sysTime.Unix()
	*shmTimeReceiveTimeStampUSec = int32(sysTime.Nanosecond() / 1e3)
	*shmTimeLeap = 0
	*shmTimePrecision = 0
	*shmTimeNSamples = 0
	*shmTimeClockTimeStampNSec = uint32(refTime.Nanosecond())
	*shmTimeReceiveTimeStampNSec = uint32(sysTime.Nanosecond())

	*shmTimeCount++
	*shmTimeValid = 1

	return nil
}
