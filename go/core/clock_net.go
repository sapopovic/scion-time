package core

import (
	"context"
	"fmt"
	"log"
	"net"
	_ "math/rand"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"

	"example.com/scion-time/go/core/timemath"
)

const netClockClientLogPrefix = "[core/clock_net]"

var errNoPaths = fmt.Errorf("failed to measure clock offset: no paths")

type NetworkClockClient struct{
	localHost *net.UDPAddr
}

func (ncc *NetworkClockClient) SetLocalHost(localHost *net.UDPAddr) {
	ncc.localHost = localHost
}

func MeasureClockOffset(localIA addr.IA, localHost *net.UDPAddr,
	peer UDPAddr, ps []snet.Path) (time.Duration, error) {
	if len(ps) == 0 {
		return 0, errNoPaths
	}
	// sp := ps[rand.Intn(len(ps))]

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
