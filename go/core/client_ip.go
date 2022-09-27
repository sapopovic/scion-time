package core

import (
	"log"
	"net"
	"time"

	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

func RunIPClient(localAddr, remoteAddr *net.UDPAddr) {
	conn, err := net.DialUDP("udp", localAddr, remoteAddr)
	if err != nil {
		log.Printf("Failed to dial UDP connection: %v", err)
		return
	}
	defer conn.Close()
	udp.EnableTimestamping(conn)

	ntpreq := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)

	cTxTime := timebase.Now()

	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)
	ntp.EncodePacket(&buf, &ntpreq)

	_, err = conn.Write(buf)
	if err != nil {
		log.Printf("Failed to write packet: %v", err)
		return
	}

	oob := make([]byte, udp.TimestampLen())

	n, oobn, flags, _, err := conn.ReadMsgUDPAddrPort(buf, oob)
	if err != nil {
		log.Printf("Failed to read packet: %v", err)
		return
	}
	if flags != 0 {
		log.Printf("Failed to read packet, flags: %v", flags)
		return
	}

	oob = oob[:oobn]
	cRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		log.Printf("Failed to receive packet timestamp")
		cRxTime = timebase.Now()
	}
	buf = buf[:n]

	var ntpresp ntp.Packet
	err = ntp.DecodePacket(&ntpresp, buf)
	if err != nil {
		log.Printf("Failed to decode packet payload: %v", err)
		return
	}

	err = ntp.ValidateResponse(&ntpresp, cTxTime)
	if err != nil {
		log.Printf("Unexpected packet received: %v", err)
		return
	}

	log.Printf("Received NTP packet: %+v", ntpresp)

	sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
	sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

	clockOffset := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	roundTripDelay := ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime)

	log.Printf("%s clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		remoteAddr,
		float64(clockOffset.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(clockOffset.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))
}
