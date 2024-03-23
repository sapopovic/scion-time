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
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
)

type Measurement struct {
	Timestamp time.Time
	Offset    time.Duration
	Error     error
}

type ReferenceClock interface {
	MeasureClockOffset(ctx context.Context) (time.Time, time.Duration, error)
}

type ReferenceClockClient struct {
	numOpsInProgress uint32
}

var (
	errNoPath             = errors.New("failed to measure clock offset: no path")
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
	ts time.Time, off time.Duration, err error) {
	mtrcs := ipMetrics.Load()

	var nerr, n int
	if ntpc.InterleavedMode {
		n = 3
	} else {
		n = 1
	}
	for i := range n {
		t, o, e := ntpc.measureClockOffsetIP(ctx, log, mtrcs, localAddr, remoteAddr)
		if e == nil {
			ts, off, err = t, o, e
			if ntpc.InInterleavedMode() {
				break
			}
		} else {
			if nerr == i {
				err = e
			}
			nerr++
			log.Info("failed to measure clock offset",
				zap.Stringer("to", remoteAddr), zap.Error(e))
		}
	}
	return
}

func collectMeasurements(ctx context.Context, ms []Measurement, msc chan Measurement) int {
	i := 0
	j := 0
	n := len(ms)
loop:
	for i != n {
		select {
		case m := <-msc:
			if m.Error == nil {
				if j != len(ms) {
					ms[j] = m
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
			<-msc
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
		return time.Time{}, 0, err
	}
	if n == 0 {
		return time.Time{}, 0, errNoPath
	}
	sps = sps[:n]

	ms := make([]Measurement, len(sps))
	msc := make(chan Measurement)
	for i := range len(sps) {
		go func(ctx context.Context, log *zap.Logger, mtrcs *scionClientMetrics,
			ntpc *SCIONClient, localAddr, remoteAddr udp.UDPAddr, p snet.Path) {
			var err error
			var ts time.Time
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
			for j := range n {
				t, o, e := ntpc.measureClockOffsetSCION(ctx, log, mtrcs, localAddr, remoteAddr, p)
				if e == nil {
					ts, off, err = t, o, e
					if ntpc.InInterleavedMode() {
						break
					}
				} else {
					if nerr == j {
						err = e
					}
					nerr++
					log.Info("failed to measure clock offset",
						zap.Stringer("to", remoteAddr.IA),
						zap.Object("via", scion.PathMarshaler{Path: p}),
						zap.Error(e),
					)
				}
			}
			msc <- Measurement{ts, off, err}
		}(ctx, log, mtrcs, ntpcs[i], localAddr, remoteAddr, sps[i])
	}
	collectMeasurements(ctx, ms, msc)
	panic("@@@")
	median := Measurement{} // timemath.Median(ms)
	return median.Timestamp, median.Offset, median.Error
}

func (c *ReferenceClockClient) MeasureClockOffsets(ctx context.Context, log *zap.Logger,
	refclks []ReferenceClock, ms []Measurement) {
	if len(ms) != len(refclks) {
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

	msc := make(chan Measurement)
	for _, refclk := range refclks {
		go func(ctx context.Context, log *zap.Logger, refclk ReferenceClock) {
			ts, off, err := refclk.MeasureClockOffset(ctx)
			msc <- Measurement{ts, off, err}
		}(ctx, log, refclk)
	}
	collectMeasurements(ctx, ms, msc)
}
