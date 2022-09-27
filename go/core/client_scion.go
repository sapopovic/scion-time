package core

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/daemon"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

func RunSCIONClient(daemonAddr string, localAddr snet.UDPAddr, remoteAddr snet.UDPAddr) {
	var err error
	ctx := context.Background()

	dc, err := daemon.NewService(daemonAddr).Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to create SCION daemon connector: %v", err)
	}

	ps, err := dc.Paths(ctx, remoteAddr.IA, localAddr.IA, daemon.PathReqFlags{Refresh: true})
	if err != nil {
		log.Fatalf("Failed to lookup paths: %v:", err)
	}
	if len(ps) == 0 {
		log.Fatalf("No paths to %v available", remoteAddr.IA)
	}

	log.Printf("Available paths to %v:", remoteAddr.IA)
	for _, p := range ps {
		log.Printf("\t%v", p)
	}

	sp := ps[0]

	log.Printf("Selected path to %v:", remoteAddr.IA)
	log.Printf("\t%v", sp)

	localAddr.Host.Port = underlay.EndhostPort

	nextHop := sp.UnderlayNextHop()
	if nextHop == nil && remoteAddr.IA.Equal(localAddr.IA) {
		nextHop = &net.UDPAddr{
			IP:   remoteAddr.Host.IP,
			Port: underlay.EndhostPort,
			Zone: remoteAddr.Host.Zone,
		}
	}

	conn, err := net.DialUDP("udp", localAddr.Host, nextHop)
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
			Path: sp.Dataplane(),
			Payload: snet.UDPPayload{
				SrcPort: uint16(localAddr.Host.Port),
				DstPort: uint16(remoteAddr.Host.Port),
				Payload: buf,
			},
		},
	}

	err = pkt.Serialize()
	if err != nil {
		log.Printf("Failed to serialize packet: %v", err)
		return
	}

	_, err = conn.Write(pkt.Bytes)
	if err != nil {
		log.Printf("Failed to write packet: %v", err)
		return
	}

	pkt.Prepare()
	oob := make([]byte, udp.TimestampLen())

	n, oobn, flags, lastHop, err := conn.ReadMsgUDPAddrPort(pkt.Bytes, oob)
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
	pkt.Bytes = pkt.Bytes[:n]

	err = pkt.Decode()
	if err != nil {
		log.Printf("Failed to decode packet: %v", err)
		return
	}

	udppkt, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		log.Printf("Failed to read packet payload: not a UDP packet")
		return
	}

	log.Printf("Received payload at %v via %v with flags = %v:", cRxTime, lastHop, flags)
	fmt.Printf("%s", hex.Dump(udppkt.Payload))

	var ntpresp ntp.Packet
	err = ntp.DecodePacket(&ntpresp, udppkt.Payload)
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

	log.Printf("%s,%s clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		remoteAddr.IA, remoteAddr.Host,
		float64(clockOffset.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(clockOffset.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))
}
