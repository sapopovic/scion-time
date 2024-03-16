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

	*p.shm.time = shmTime{
		mode:                 0,
		clockTimeStampSec:    refTime.Unix(),
		clockTimeStampUSec:   int32(refTime.Nanosecond() / 1e3),
		receiveTimeStampSec:  sysTime.Unix(),
		receiveTimeStampUSec: int32(sysTime.Nanosecond() / 1e3),
		leap:                 0,
		precision:            0,
		nSamples:             0,
		clockTimeStampNSec:   uint32(refTime.Nanosecond()),
		receiveTimeStampNSec: uint32(sysTime.Nanosecond()),
		count:                p.shm.time.count + 1,
		valid:                1,
	}

	return nil
}
