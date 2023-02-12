package core

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/snet"

	"go.uber.org/zap"

	"example.com/scion-time/go/core/crypto"
	"example.com/scion-time/go/core/timemath"
	"example.com/scion-time/go/net/scion"
	"example.com/scion-time/go/net/udp"

	"example.com/scion-time/go/driver/ntp"
)

type measurement struct {
	off time.Duration
	err error
}

type ReferenceClock interface {
	MeasureClockOffset(ctx context.Context) (time.Duration, error)
	String() string
}

type ReferenceClockClient struct {
	Log              *zap.Logger
	numOpsInProgress uint32
}

var (
	errNoPaths = errors.New("failed to measure clock offset: no paths")
)

func MeasureClockOffsetIP(ctx context.Context, ntpc *ntp.IPClient,
	localAddr, remoteAddr *net.UDPAddr) (time.Duration, error) {
	var err error
	var off time.Duration
	var nerr, n int
	if ntpc.InterleavedMode {
		n = 2
	} else {
		n = 1
	}
	for i := 0; i != n; i++ {
		o, _, e := ntpc.MeasureClockOffsetIP(ctx, localAddr, remoteAddr)
		if e == nil {
			off, err = o, e
		} else {
			if nerr == i {
				off, err = o, e
			}
			nerr++
			ntpc.Log.Info("failed to measure clock offset",
				zap.Stringer("from", remoteAddr), zap.Error(e))
		}
	}
	return off, err
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

func MeasureClockOffsetSCION(ctx context.Context, ntpc *ntp.SCIONClient,
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
	ntpc.Log.Debug("selected paths", zap.Stringer("to", remoteAddr.IA), zap.Array("via", scion.PathArrayMarshaler{Paths: ps}))

	off := make([]time.Duration, len(sps))
	ms := make(chan measurement)
	var ntpcMu sync.Mutex
	for _, p := range sps {
		go func(ctx context.Context, ntpc *ntp.SCIONClient,
			localAddr, remoteAddr udp.UDPAddr, p snet.Path) {
			var err error
			var off time.Duration
			var nerr, n int
			if ntpc.InterleavedMode {
				n = 2
			} else {
				n = 1
			}
			for i := 0; i != n; i++ {
				// TODO: find a way to parallelize measurements over multiple paths in interleaved mode
				o, _, e := func() (time.Duration, float64, error) {
					ntpcMu.Lock()
					defer ntpcMu.Unlock()
					return ntpc.MeasureClockOffsetSCION(ctx, localAddr, remoteAddr, p)
				}()
				if e == nil {
					off, err = o, e
				} else {
					if nerr == i {
						off, err = o, e
					}
					nerr++
					ntpc.Log.Info("failed to measure clock offset",
						zap.Stringer("from", remoteAddr.IA), zap.Any("via", p), zap.Error(e))
				}
			}
			ms <- measurement{off, err}
		}(ctx, ntpc, localAddr, remoteAddr, p)
	}
	collectMeasurements(ctx, off, ms)
	return timemath.Median(off), nil
}

func (c *ReferenceClockClient) MeasureClockOffsets(ctx context.Context,
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
			off, err := refclk.MeasureClockOffset(ctx)
			if err != nil {
				log.Info("failed to measure clock offset",
					zap.Stringer("from", refclk),
					zap.Error(err),
				)
			}
			ms <- measurement{off, err}
		}(ctx, c.Log, refclk)
	}
	collectMeasurements(ctx, off, ms)
}
