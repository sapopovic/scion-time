package core

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"

	"example.com/scion-time/go/core/crypto"
	"example.com/scion-time/go/core/timemath"
)

const netClockClientLogPrefix = "[core/clock_net]"

var errNoPaths = fmt.Errorf("failed to measure clock offset: no paths")

type NetworkClockClient struct {
	localHost *net.UDPAddr
}

func (ncc *NetworkClockClient) SetLocalHost(localHost *net.UDPAddr) {
	ncc.localHost = localHost
}

func MeasureClockOffset(localIA addr.IA, localHost *net.UDPAddr,
	peer UDPAddr, ps []snet.Path) (time.Duration, error) {
	sp := make([]snet.Path, 5)
	n, err := crypto.Sample(context.TODO(), len(sp), len(ps), func(dst, src int) {
		sp[dst] = ps[src]
	})
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, errNoPaths
	}
	sp = sp[:n]

	for _, p := range sp {
		log.Printf("%s Selected path to %v: %v", netClockClientLogPrefix, peer.IA, p)
	}

	panic("not yet implemented")

	return 0, nil
}

func (ncc *NetworkClockClient) MeasureClockOffset(ctx context.Context,
	peers []UDPAddr, pi PathInfo) (time.Duration, error) {
	type measurement struct {
		off time.Duration
		err error
	}
	ms := make(chan measurement)
	for _, p := range peers {
		go func(localIA addr.IA, localHost *net.UDPAddr, peer UDPAddr, paths []snet.Path) {
			off, err := MeasureClockOffset(localIA, localHost, peer, paths)
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v: %v", netClockClientLogPrefix, peer.IA, err)
			}
			ms <- measurement{off, err}
		}(pi.LocalIA, ncc.localHost, p, pi.Paths[p.IA])
	}
	i := 0
	var off []time.Duration
loop:
	for i != len(peers) {
		select {
		case m := <-ms:
			if m.err == nil {
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
	}(len(peers) - i)
	if len(off) == 0 {
		return 0, errNoClockMeasurements
	}
	return timemath.FaultTolerantMidpoint(off), nil
}
