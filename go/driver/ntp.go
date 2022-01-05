package drivers

import (
	"log"
	"net"
	"time"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const ntpLogPrefix = "[drivers/ntp]"

func FetchNTPTime(host string) (refTime time.Time, sysTime time.Time, err error) {
	refTime = time.Time{}
	sysTime = time.Time{}

	timeout := 5 * time.Second
	now := time.Now().UTC()
	deadline := now.Add(timeout)
	addr := net.JoinHostPort(host, "123")
	conn, err := net.DialTimeout("udp", addr, deadline.Sub(now))
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetDeadline(deadline)
	udpConn := conn.(*net.UDPConn)

	err = udp.EnableTimestamping(udpConn)
	if err != nil {
		return
	}

	pkt := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)
	oob := make([]byte, udp.TimestampOutOfBandDataLen())

	clientTxTime := time.Now().UTC()

	pkt.SetVersion(ntp.VersionMax)
	pkt.SetMode(ntp.ModeClient)
	pkt.TransmitTime = ntp.Time64FromTime(clientTxTime)
	ntp.EncodePacket(&buf, &pkt)

	_, err = udpConn.Write(buf)
	if err != nil {
		return
	}
	n, oobn, _, _, err := udpConn.ReadMsgUDP(buf, oob)
	if err != nil {
		return
	}

	oob = oob[:oobn]
	clientRxTime, err := udp.TimeFromOutOfBandData(oob)
	if err != nil {
		log.Printf("%s %s, failed to read packet timestamp", ntpLogPrefix, host, err)
		clientRxTime = time.Now().UTC()
	}

	buf = buf[:n]
	err = ntp.DecodePacket(&pkt, buf)
	if err != nil {
		log.Printf("%s %s, failed to decode packet payload: %v", ntpLogPrefix, host, err)
		return
	}

	log.Printf("%s %s, received packet: %+v", ntpLogPrefix, host, pkt)

	serverRxTime := ntp.TimeFromTime64(pkt.ReceiveTime)
	serverTxTime := ntp.TimeFromTime64(pkt.TransmitTime)

	clockOffset := ntp.ClockOffset(clientTxTime, serverRxTime, serverTxTime, clientRxTime)
	roundTripDelay := ntp.RoundTripDelay(clientTxTime, serverRxTime, serverTxTime, clientRxTime)

	log.Printf("%s %s, clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		ntpLogPrefix, host,
		float64(clockOffset.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(clockOffset.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	sysTime = clientRxTime
	refTime = clientRxTime.Add(clockOffset)
	return
}
