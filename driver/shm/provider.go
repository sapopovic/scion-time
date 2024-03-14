package shm

import (
	"time"

	"go.uber.org/zap"
)

type Provider struct {
	unit int
	shm  segment
}

func NewProvider(unit int) *Provider {
	return &Provider{unit: unit}
}

func (p *Provider) StoreClockSamples(log *zap.Logger, refTime, sysTime time.Time) error {
	if !p.shm.initialized {
		err := initSegment(&p.shm, p.unit)
		if err != nil {
			return err
		}
	}

	*p.shm.timeMode = 0
	*p.shm.timeClockTimeStampSec = refTime.Unix()
	*p.shm.timeClockTimeStampUSec = int32(refTime.Nanosecond() / 1e3)
	*p.shm.timeReceiveTimeStampSec = sysTime.Unix()
	*p.shm.timeReceiveTimeStampUSec = int32(sysTime.Nanosecond() / 1e3)
	*p.shm.timeLeap = 0
	*p.shm.timePrecision = 0
	*p.shm.timeNSamples = 0
	*p.shm.timeClockTimeStampNSec = uint32(refTime.Nanosecond())
	*p.shm.timeReceiveTimeStampNSec = uint32(sysTime.Nanosecond())

	*p.shm.timeCount++
	*p.shm.timeValid = 1

	return nil
}
