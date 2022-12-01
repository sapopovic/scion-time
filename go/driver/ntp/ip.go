package ntp

import (
	"context"
	"log"
	"net"
	"time"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

func MeasureClockOffsetIP(ctx context.Context, localAddr, remoteAddr *net.UDPAddr) (
	offset time.Duration, weight float64, err error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localAddr.IP})
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

	ntpreq := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)

	cTxTime := timebase.Now()

	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)
	ntp.EncodePacket(&buf, &ntpreq)

	_, err = conn.WriteToUDPAddrPort(buf, remoteAddr.AddrPort())
	if err != nil {
		return offset, weight, err
	}

	oob := make([]byte, udp.TimestampLen())
	n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
	if err != nil {
		return offset, weight, err
	}
	if flags != 0 {
		return offset, weight, errUnexpectedPacketFlags
	}

	oob = oob[:oobn]
	cRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		cRxTime = timebase.Now()
		log.Printf("%s Failed to receive packet timestamp: %v", ntpLogPrefix, err)
	}
	buf = buf[:n]

	var ntpresp ntp.Packet
	err = ntp.DecodePacket(&ntpresp, buf)
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

	log.Printf("%s %s, clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		ntpLogPrefix, remoteAddr,
		float64(off.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(off.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	// offset, weight = off, 1000.0

	reference := remoteAddr.String()
	offset, weight = filter(reference, cTxTime, sRxTime, sTxTime, cRxTime)

	return offset, weight, nil
}
