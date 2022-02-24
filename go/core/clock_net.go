package core

import (
	"context"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"
)

type NetworkClockClient struct{}

func (ncc *NetworkClockClient) MeasureClockOffset(ctx context.Context, pi PathInfo) (time.Duration, error) {
	type measurement struct {
		off time.Duration
		err error
	}
	ms := make(chan measurement)
	for peerIA, paths := range pi.PeerIAPaths {
		go func(peerIA addr.IA, paths []snet.Path) {
			panic("not yet implemented")
			ms <- measurement{0, nil}
		}(peerIA, paths)
	}
	i := 0
	var off []time.Duration
loop:
	for i != len(pi.PeerIAPaths) {
		select {
		case m := <-ms:
			if m.err != nil {
				off = append(off, m.off)
			}
			i++
		case <-ctx.Done():
			break loop
		}
	}
	go func(n int) { // drain ms
		for n != 0 {
			<-ms
			n--
		}
	}(len(pi.PeerIAPaths) - i)
	if len(off) == 0 {
		return 0, errNoClockMeasurements
	}
	return FaultTolerantMidpoint(off), nil
}
