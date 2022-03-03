package ntp

import (
	"fmt"
	"log"
	"net"
	"time"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const ntpLogPrefix = "[driver/ntp]"

var errUnexpectedPacketFlags = fmt.Errorf("failed to read packet: unexpected flags")

func MeasureClockOffset(host string) (time.Duration, error) {
	timeout := 5 * time.Second
	now := time.Now().UTC()
	deadline := now.Add(timeout)
	addr := net.JoinHostPort(host, "123")
	conn, err := net.DialTimeout("udp", addr, deadline.Sub(now))
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	conn.SetDeadline(deadline)
	udpConn := conn.(*net.UDPConn)
	udp.EnableTimestamping(udpConn)

	pkt := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)
	oob := make([]byte, udp.TimestampLen())

	clientTxTime := time.Now().UTC()

	pkt.SetVersion(ntp.VersionMax)
	pkt.SetMode(ntp.ModeClient)
	pkt.TransmitTime = ntp.Time64FromTime(clientTxTime)
	ntp.EncodePacket(&buf, &pkt)

	_, err = udpConn.Write(buf)
	if err != nil {
		return 0, err
	}
	n, oobn, flags, srcAddr, err := udpConn.ReadMsgUDP(buf, oob)
	if err != nil {
		log.Printf("%s Failed to read packet: %v", ntpLogPrefix, err)
		return 0, err
	}
	if flags != 0 {
		log.Printf("%s Failed to read packet, flags: %v", ntpLogPrefix, flags)
		return 0, errUnexpectedPacketFlags
	}

	oob = oob[:oobn]
	clientRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		log.Printf("%s %s, failed to read packet timestamp", ntpLogPrefix, host, err)
		clientRxTime = time.Now().UTC()
	}
	buf = buf[:n]

	err = ntp.DecodePacket(&pkt, buf)
	if err != nil {
		log.Printf("%s %s, failed to decode packet payload: %v", ntpLogPrefix, host, err)
		return 0, err
	}

	log.Printf("%s %s, received packet at %v from srcAddr: %+v", ntpLogPrefix, host, pkt, clientRxTime, srcAddr)

	serverRxTime := ntp.TimeFromTime64(pkt.ReceiveTime)
	serverTxTime := ntp.TimeFromTime64(pkt.TransmitTime)

	off := ntp.ClockOffset(clientTxTime, serverRxTime, serverTxTime, clientRxTime)
	rtd := ntp.RoundTripDelay(clientTxTime, serverRxTime, serverTxTime, clientRxTime)

	log.Printf("%s %s, clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		ntpLogPrefix, host,
		float64(off.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(off.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	return off, nil
}
