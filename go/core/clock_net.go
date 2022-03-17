package core

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	"example.com/scion-time/go/core/crypto"
	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/core/timemath"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const netClockClientLogPrefix = "[core/clock_net]"

var (
	errNoPaths                 = fmt.Errorf("failed to measure clock offset: no paths")
	errUnexpectedPacketFlags   = fmt.Errorf("failed to read packet: unexpected flags")
	errUnexpectedPacketPayload = fmt.Errorf("failed to read packet: unexpected payload")
)

type NetworkClockClient struct {
	localHost        *net.UDPAddr
	numOpsInProgress uint32
}

func (ncc *NetworkClockClient) SetLocalHost(localHost *net.UDPAddr) {
	ncc.localHost = localHost
}

func measureClockOffsetViaPath(ctx context.Context,
	localAddr, peerAddr UDPAddr, p snet.Path) (time.Duration, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localAddr.Host.IP})
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if ok {
		err := conn.SetDeadline(deadline)
		if err != nil {
			return 0, err
		}
	}
	udp.EnableTimestamping(conn)

	localAddr.Host.Port = conn.LocalAddr().(*net.UDPAddr).Port

	nextHop := p.UnderlayNextHop().AddrPort()
	if nextHop == (netip.AddrPort{}) && peerAddr.IA.Equal(localAddr.IA) {
		nextHop = netip.AddrPortFrom(
			peerAddr.Host.AddrPort().Addr(),
			underlay.EndhostPort)
	}

	ntpreq := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)

	cTxTime := timebase.Now()

	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)
	ntp.EncodePacket(&buf, &ntpreq)

	pkt := &snet.Packet{
		PacketInfo: snet.PacketInfo{
			Source: snet.SCIONAddress{
				IA:   localAddr.IA,
				Host: addr.HostFromIP(localAddr.Host.IP),
			},
			Destination: snet.SCIONAddress{
				IA:   peerAddr.IA,
				Host: addr.HostFromIP(peerAddr.Host.IP),
			},
			Path: p.Dataplane(),
			Payload: snet.UDPPayload{
				SrcPort: uint16(localAddr.Host.Port),
				DstPort: uint16(peerAddr.Host.Port),
				Payload: buf,
			},
		},
	}

	err = pkt.Serialize()
	if err != nil {
		return 0, err
	}

	_, err = conn.WriteToUDPAddrPort(pkt.Bytes, nextHop)
	if err != nil {
		return 0, err
	}

	pkt.Prepare()
	oob := make([]byte, udp.TimestampLen())

	n, oobn, flags, _, err := conn.ReadMsgUDPAddrPort(pkt.Bytes, oob)
	if err != nil {
		return 0, err
	}
	if flags != 0 {
		return 0, errUnexpectedPacketFlags
	}

	oob = oob[:oobn]
	cRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		log.Printf("%s Failed to receive packet timestamp", netClockClientLogPrefix)
		cRxTime = timebase.Now()
	}
	pkt.Bytes = pkt.Bytes[:n]

	err = pkt.Decode()
	if err != nil {
		return 0, err
	}

	udppkt, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		return 0, errUnexpectedPacketPayload
	}

	var ntpresp ntp.Packet
	err = ntp.DecodePacket(&ntpresp, udppkt.Payload)
	if err != nil {
		return 0, err
	}

	log.Printf("%s Received NTP packet: %+v", netClockClientLogPrefix, ntpresp)

	sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
	sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

	clockOffset := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	roundTripDelay := ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime)

	log.Printf("%s %s,%s clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		netClockClientLogPrefix, peerAddr.IA, peerAddr.Host,
		float64(clockOffset.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(clockOffset.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	return clockOffset, nil
}

func measureClockOffsetToPeer(ctx context.Context,
	localAddr, peerAddr UDPAddr, ps []snet.Path) (time.Duration, error) {
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
		log.Printf("%s Selected path to %v: %v", netClockClientLogPrefix, peerAddr.IA, p)
	}

	ms := make(chan measurement)
	for _, p := range sps {
		go func(ctx context.Context, localAddr, peerAddr UDPAddr, p snet.Path) {
			off, err := measureClockOffsetViaPath(ctx, localAddr, peerAddr, p)
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v via %v: %v",
					netClockClientLogPrefix, peerAddr.IA, p, err)
			}
			ms <- measurement{off, err}
		}(ctx, localAddr, peerAddr, p)
	}
	off := collectMeasurements(ctx, ms, len(sps))
	if len(off) == 0 {
		return 0, errNoClockMeasurements
	}
	return timemath.Median(off), nil
}

func (ncc *NetworkClockClient) MeasureClockOffset(ctx context.Context,
	peers []UDPAddr, pi PathInfo) (time.Duration, error) {
	swapped := atomic.CompareAndSwapUint32(&ncc.numOpsInProgress, 0, 1)
	if !swapped {
		panic("too many network clock offset measurements in progress")
	}
	defer func(addr *uint32) {
		swapped := atomic.CompareAndSwapUint32(addr, 1, 0)
		if !swapped {
			panic("inconsistent count of network clock offset measurements")
		}
	}(&ncc.numOpsInProgress)

	ms := make(chan measurement)
	for _, p := range peers {
		go func(ctx context.Context, localAddr, peerAddr UDPAddr, ps []snet.Path) {
			off, err := measureClockOffsetToPeer(ctx, localAddr, peerAddr, ps)
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v: %v",
					netClockClientLogPrefix, peerAddr.IA, err)
			}
			ms <- measurement{off, err}
		}(ctx, UDPAddr{pi.LocalIA, ncc.localHost}, p, pi.Paths[p.IA])
	}
	off := collectMeasurements(ctx, ms, len(peers))
	if len(off) == 0 {
		return 0, errNoClockMeasurements
	}
	return timemath.FaultTolerantMidpoint(off), nil
}
