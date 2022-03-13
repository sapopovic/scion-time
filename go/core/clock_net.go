package core

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	"example.com/scion-time/go/core/crypto"
	"example.com/scion-time/go/core/timemath"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const netClockClientLogPrefix = "[core/clock_net]"

var (
	errNoPaths = fmt.Errorf("failed to measure clock offset: no paths")
	errUnexpectedPacketFlags = fmt.Errorf("failed to read packet: unexpected flags")
	errUnexpectedPacketPayload = fmt.Errorf("failed to read packet: unexpected payload")
)

type measurement struct {
	off time.Duration
	err error
}

type NetworkClockClient struct {
	localHost *net.UDPAddr
}

func (ncc *NetworkClockClient) SetLocalHost(localHost *net.UDPAddr) {
	ncc.localHost = localHost
}

func collectMeasurements(ctx context.Context, ms chan measurement, n int) []time.Duration {
	i := 0
	var off []time.Duration
loop:
	for i != n {
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
	go func(n int) { // drain channel
		for n != 0 {
			<-ms
			n--
		}
	}(n - i)
	return off
}

func measureClockOffsetViaPath(ctx context.Context,
	localIA addr.IA, localHost *net.UDPAddr,
	peer UDPAddr, p snet.Path) (time.Duration, error) {

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localHost.IP})
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	udp.EnableTimestamping(conn)

	localHost.Port = conn.LocalAddr().(*net.UDPAddr).Port

	nextHop := p.UnderlayNextHop()
	if nextHop == nil && peer.IA.Equal(localIA) {
		nextHop = &net.UDPAddr{
			IP:   peer.Host.IP,
			Port: underlay.EndhostPort,
			Zone: peer.Host.Zone,
		}
	}

	ntpreq := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)

	cTxTime := time.Now().UTC()

	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)
	ntp.EncodePacket(&buf, &ntpreq)

	pkt := &snet.Packet{
		PacketInfo: snet.PacketInfo{
			Source: snet.SCIONAddress{
				IA:   localIA,
				Host: addr.HostFromIP(localHost.IP),
			},
			Destination: snet.SCIONAddress{
				IA:   peer.IA,
				Host: addr.HostFromIP(peer.Host.IP),
			},
			Path: p.Dataplane(),
			Payload: snet.UDPPayload{
				SrcPort: uint16(localHost.Port),
				DstPort: uint16(peer.Host.Port),
				Payload: buf,
			},
		},
	}

	err = pkt.Serialize()
	if err != nil {
		return 0, err
	}

	_, err = conn.WriteTo(pkt.Bytes, nextHop)
	if err != nil {
		return 0, err
	}

	pkt.Prepare()
	oob := make([]byte, udp.TimestampLen())

	n, oobn, flags, _, err := conn.ReadMsgUDP(pkt.Bytes, oob)
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
		cRxTime = time.Now().UTC()
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
		netClockClientLogPrefix, peer.IA, peer.Host,
		float64(clockOffset.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(clockOffset.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	return clockOffset, nil
}

func measureClockOffsetToPeer(ctx context.Context,
	localIA addr.IA, localHost *net.UDPAddr,
	peer UDPAddr, ps []snet.Path) (time.Duration, error) {
	sp := make([]snet.Path, 5)
	n, err := crypto.Sample(ctx, len(sp), len(ps), func(dst, src int) {
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

	ms := make(chan measurement)
	for _, p := range sp {
		go func(ctx context.Context, localIA addr.IA, localHost *net.UDPAddr,
			peer UDPAddr, p snet.Path) {
			off, err := measureClockOffsetViaPath(ctx, localIA, localHost, peer, p)
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v via %v: %v",
					netClockClientLogPrefix, peer.IA, p, err)
			}
			ms <- measurement{off, err}
		}(ctx, localIA, localHost, peer, p)
	}
	off := collectMeasurements(ctx, ms, len(sp))
	if len(off) == 0 {
		return 0, errNoClockMeasurements
	}
	return timemath.Median(off), nil
}

func (ncc *NetworkClockClient) MeasureClockOffset(ctx context.Context,
	peers []UDPAddr, pi PathInfo) (time.Duration, error) {
	ms := make(chan measurement)
	for _, p := range peers {
		go func(ctx context.Context, localIA addr.IA, localHost *net.UDPAddr,
			peer UDPAddr, ps []snet.Path) {
			off, err := measureClockOffsetToPeer(ctx, localIA, localHost, peer, ps)
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v: %v",
					netClockClientLogPrefix, peer.IA, err)
			}
			ms <- measurement{off, err}
		}(ctx, pi.LocalIA, ncc.localHost, p, pi.Paths[p.IA])
	}
	off := collectMeasurements(ctx, ms, len(peers))
	if len(off) == 0 {
		return 0, errNoClockMeasurements
	}
	return timemath.FaultTolerantMidpoint(off), nil
}
