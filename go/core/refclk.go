package core

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/snet"

	"go.uber.org/zap"

	"example.com/scion-time/go/core/crypto"
	"example.com/scion-time/go/core/timemath"
	"example.com/scion-time/go/net/udp"

	"example.com/scion-time/go/driver/ntp"
)

type measurement struct {
	off time.Duration
	err error
}

type ReferenceClock interface {
	MeasureClockOffset(ctx context.Context) (time.Duration, error)
}

type ReferenceClockClient struct {
	Log              *zap.Logger
	numOpsInProgress uint32
}

var (
	errNoPaths = errors.New("failed to measure clock offset: no paths")
)

func collectMeasurements(ctx context.Context, off []time.Duration, ms chan measurement, n int) int {
	i := 0
	j := 0
loop:
	for i != n {
		select {
		case m := <-ms:
			if m.err == nil {
				if j != len(off) {
					off[j] = m.off
					j++
				}
			}
			i++
		case <-ctx.Done():
			break loop
		}
	}
	go func(n int) { // drain channel
		for n != 0 {
			<-ms
			n--
		}
	}(n - i)
	return j
}

func MeasureClockOffsetSCION(ctx context.Context, log *zap.Logger,
	localAddr, remoteAddr udp.UDPAddr, ps []snet.Path) (time.Duration, error) {
	sps := make([]snet.Path, 5)
	n, err := crypto.Sample(ctx, len(sps), len(ps), func(dst, src int) {
		sps[dst] = ps[src]
	})
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, errNoPaths
	}
	sps = sps[:n]
	log.Debug("selected paths", zap.Stringer("to", remoteAddr.IA), zap.Any("via", sps))

	off := make([]time.Duration, len(sps))

	ms := make(chan measurement)
	for _, p := range sps {
		go func(ctx context.Context, log *zap.Logger,
			localAddr, remoteAddr udp.UDPAddr, p snet.Path) {
			off, _, err := ntp.MeasureClockOffsetSCION(ctx, log, localAddr, remoteAddr, p)
			if err != nil {
				log.Info("failed to fetch clock offset",
					zap.Stringer("from", remoteAddr.IA),
					zap.Any("via", p),
					zap.Error(err),
				)
			}
			ms <- measurement{off, err}
		}(ctx, log, localAddr, remoteAddr, p)
	}
	collectMeasurements(ctx, off, ms, len(sps))
	return timemath.Median(off), nil
}

func (c *ReferenceClockClient) MeasureClockOffsets(ctx context.Context,
	refclks []ReferenceClock, off []time.Duration) {
	swapped := atomic.CompareAndSwapUint32(&c.numOpsInProgress, 0, 1)
	if !swapped {
		panic("too many reference clock offset measurements in progress")
	}
	defer func(addr *uint32) {
		swapped := atomic.CompareAndSwapUint32(addr, 1, 0)
		if !swapped {
			panic("inconsistent count of reference clock offset measurements")
		}
	}(&c.numOpsInProgress)

	ms := make(chan measurement)
	for _, refclk := range refclks {
		go func(ctx context.Context, log *zap.Logger, refclk ReferenceClock) {
			off, err := refclk.MeasureClockOffset(ctx)
			if err != nil {
				log.Info("failed to fetch clock offset",
					zap.Any("from", refclk),
					zap.Error(err),
				)
			}
			ms <- measurement{off, err}
		}(ctx, c.Log, refclk)
	}
	collectMeasurements(ctx, off, ms, len(refclks))
}
