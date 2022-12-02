package core

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/snet"

	"example.com/scion-time/go/core/crypto"
	"example.com/scion-time/go/core/timemath"
	"example.com/scion-time/go/net/udp"

	"example.com/scion-time/go/driver/ntp"
)

const clockClientLogPrefix = "[core/clock_client]"

type measurement struct {
	off time.Duration
	err error
}

type ReferenceClock interface {
	MeasureClockOffset(ctx context.Context) (time.Duration, error)
}

type ReferenceClockClient struct {
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

func MeasureClockOffsetSCION(ctx context.Context,
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
	for _, p := range sps {
		log.Printf("%s Selected path to %v: %v", clockClientLogPrefix, remoteAddr.IA, p)
	}
	off := make([]time.Duration, len(sps))

	ms := make(chan measurement)
	for _, p := range sps {
		go func(ctx context.Context, localAddr, remoteAddr udp.UDPAddr, p snet.Path) {
			off, _, err := ntp.MeasureClockOffsetSCION(ctx, localAddr, remoteAddr, p)
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v via %v: %v",
					clockClientLogPrefix, remoteAddr.IA, p, err)
			}
			ms <- measurement{off, err}
		}(ctx, localAddr, remoteAddr, p)
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
		go func(ctx context.Context, refclk ReferenceClock) {
			off, err := refclk.MeasureClockOffset(ctx)
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v: %v",
					clockClientLogPrefix, refclk, err)
			}
			ms <- measurement{off, err}
		}(ctx, refclk)
	}
	collectMeasurements(ctx, off, ms, len(refclks))
}
