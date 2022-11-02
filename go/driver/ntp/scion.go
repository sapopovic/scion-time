package ntp

import (
	"context"
	"log"
	"net"
	"net/netip"
	"time"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/private/topology/underlay"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

func MeasureClockOffsetSCION(ctx context.Context, localAddr, remoteAddr udp.UDPAddr,
	path snet.Path) (offset time.Duration, weight float64, err error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localAddr.Host.IP})
	if err != nil {
		return offset, weight, err
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if ok {
		err = conn.SetDeadline(deadline)
		if err != nil {
			return offset, weight, err
		}
	}
	udp.EnableTimestamping(conn)

	localPort := conn.LocalAddr().(*net.UDPAddr).Port

	nextHop := path.UnderlayNextHop().AddrPort()
	if nextHop.Addr().Is4In6() {
		nextHop = netip.AddrPortFrom(
			netip.AddrFrom4(nextHop.Addr().As4()),
			nextHop.Port())
	}
	if nextHop == (netip.AddrPort{}) && remoteAddr.IA.Equal(localAddr.IA) {
		nextHop = netip.AddrPortFrom(
			remoteAddr.Host.AddrPort().Addr(),
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
				IA:   remoteAddr.IA,
				Host: addr.HostFromIP(remoteAddr.Host.IP),
			},
			Path: path.Dataplane(),
			Payload: snet.UDPPayload{
				SrcPort: uint16(localPort),
				DstPort: uint16(remoteAddr.Host.Port),
				Payload: buf,
			},
		},
	}

	err = pkt.Serialize()
	if err != nil {
		return offset, weight, err
	}

	_, err = conn.WriteToUDPAddrPort(pkt.Bytes, nextHop)
	if err != nil {
		return offset, weight, err
	}

	pkt.Prepare()

	oob := make([]byte, udp.TimestampLen())
	n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(pkt.Bytes, oob)
	if err != nil {
		return offset, weight, err
	}
	if flags != 0 {
		return offset, weight, errUnexpectedPacketFlags
	}

	oob = oob[:oobn]
	cRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		log.Printf("%s Failed to receive packet timestamp: %v", ntpLogPrefix, err)
		cRxTime = timebase.Now()
	}
	pkt.Bytes = pkt.Bytes[:n]

	err = pkt.Decode()
	if err != nil {
		return offset, weight, err
	}

	udppkt, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		return offset, weight, errUnexpectedPacketPayload
	}

	var ntpresp ntp.Packet
	err = ntp.DecodePacket(&ntpresp, udppkt.Payload)
	if err != nil {
		return offset, weight, err
	}

	err = ntp.ValidateResponse(&ntpresp, cTxTime)
	if err != nil {
		return offset, weight, err
	}

	log.Printf("%s Received packet at %v from %v: %+v", ntpLogPrefix, cRxTime, srcAddr, ntpresp)

	sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
	sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

	off := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	rtd := ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime)

	log.Printf("%s %s,%s clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		ntpLogPrefix, remoteAddr.IA, remoteAddr.Host,
		float64(off.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(off.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	// offset, weight = off, 1000.0

	reference := remoteAddr.IA.String() + "," + remoteAddr.Host.String()
	offset, weight = filter(reference, cTxTime, sRxTime, sTxTime, cRxTime)

	return offset, weight, nil
}
