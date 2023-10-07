package client

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/snet"

	"go.uber.org/zap"

	"example.com/scion-time/base/crypto"
	"example.com/scion-time/base/timemath"
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
)

type measurement struct {
	off time.Duration
	err error
}

type ReferenceClock interface {
	MeasureClockOffset(ctx context.Context, log *zap.Logger) (time.Duration, error)
}

type ReferenceClockClient struct {
	numOpsInProgress uint32
}

var (
	errNoPaths            = errors.New("failed to measure clock offset: no paths")
	errUnexpectedAddrType = errors.New("unexpected address type")

	ipMetrics    atomic.Pointer[ipClientMetrics]
	scionMetrics atomic.Pointer[scionClientMetrics]
)

func init() {
	ipMetrics.Store(newIPClientMetrics())
	scionMetrics.Store(newSCIONClientMetrics())
}

func MeasureClockOffsetIP(ctx context.Context, log *zap.Logger,
	ntpc *IPClient, localAddr, remoteAddr *net.UDPAddr) (
	time.Time, time.Duration, error) {
	mtrcs := ipMetrics.Load()

	var err error
	var at time.Time
	var off time.Duration
	var nerr, n int
	if ntpc.InterleavedMode {
		n = 3
	} else {
		n = 1
	}
	for i := 0; i != n; i++ {
		t, o, _, e := ntpc.measureClockOffsetIP(ctx, log, mtrcs, localAddr, remoteAddr)
		if e == nil {
			at, off, err = t, o, e
		} else {
			if nerr == i {
				at, off, err = t, o, e
			}
			nerr++
			log.Info("failed to measure clock offset",
				zap.Stringer("to", remoteAddr), zap.Error(e))
		}
	}
	return at, off, err
}

func collectMeasurements(ctx context.Context, off []time.Duration, ms chan measurement) int {
	i := 0
	j := 0
	n := len(off)
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
	ntpcs []*SCIONClient, localAddr, remoteAddr udp.UDPAddr, ps []snet.Path) (
	time.Time, time.Duration, error) {
	mtrcs := scionMetrics.Load()

	sps := make([]snet.Path, len(ntpcs))
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

	off := make([]time.Duration, len(sps))
	ms := make(chan measurement)
	for i := 0; i != len(sps); i++ {
		go func(ctx context.Context, log *zap.Logger, mtrcs *scionClientMetrics,
			ntpc *SCIONClient, localAddr, remoteAddr udp.UDPAddr, p snet.Path) {
			var err error
			var off time.Duration
			var nerr, n int
			log.Debug("measuring clock offset",
				zap.Stringer("to", remoteAddr.IA),
				zap.Object("via", scion.PathMarshaler{Path: p}),
			)
			if ntpc.InterleavedMode {
				ntpc.ResetInterleavedMode()
				n = 3
			} else {
				n = 1
			}
			for j := 0; j != n; j++ {
				_, o, _, e := ntpc.measureClockOffsetSCION(ctx, log, mtrcs, localAddr, remoteAddr, p)
				if e == nil {
					off, err = o, e
				} else {
					if nerr == j {
						off, err = o, e
					}
					nerr++
					log.Info("failed to measure clock offset",
						zap.Stringer("to", remoteAddr.IA),
						zap.Object("via", scion.PathMarshaler{Path: p}),
						zap.Error(e),
					)
				}
			}
			ms <- measurement{off, err}
		}(ctx, log, mtrcs, ntpcs[i], localAddr, remoteAddr, sps[i])
	}
	collectMeasurements(ctx, off, ms)
	return timemath.Median(off), nil
}

func (c *ReferenceClockClient) MeasureClockOffsets(ctx context.Context, log *zap.Logger,
	refclks []ReferenceClock, off []time.Duration) {
	if len(off) != len(refclks) {
		panic("number of result offsets must be equal to the number of reference clocks")
	}
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
			off, err := refclk.MeasureClockOffset(ctx, log)
			ms <- measurement{off, err}
		}(ctx, log, refclk)
	}
	collectMeasurements(ctx, off, ms)
}
