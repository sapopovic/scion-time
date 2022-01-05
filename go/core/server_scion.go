package core

import (
	"unsafe"

	"encoding/hex"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/sys/unix"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const scionServerLogPrefix = "[core/server_scion]"

func StartSCIONServer(localIA addr.IA, localHost *net.UDPAddr) error {
	log.Printf("%s Listening in %v on %v:%d via SCION", scionServerLogPrefix, localIA, localHost.IP, localHost.Port)

	localHostPort := localHost.Port
	localHost.Port = underlay.EndhostPort

	conn, err := net.ListenUDP("udp", localHost)
	if err != nil {
		log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
	}
	defer conn.Close()
	err = udp.EnableTimestamping(conn)
	if err != nil {
		log.Fatalf("%s Failed to enable kernel timestamping for packets: %v", scionServerLogPrefix, err)
	}

	var pkt snet.Packet
	var udppkt snet.UDPPayload
	oob := make([]byte, udp.TimestampControlMessageLen)
	for {
		pkt.Prepare()
		oob = oob[:cap(oob)]

		n, oobn, flags, lastHop, err := conn.ReadMsgUDP(pkt.Bytes, oob)
		if err != nil {
			log.Printf("%s Failed to read packet: %v", scionServerLogPrefix, err)
			continue
		}

		var rxt time.Time
		if oobn != 0 {
			ts := (*unix.Timespec)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
			rxt = time.Unix(ts.Unix())
		} else {
			log.Printf("%s Failed to receive packet timestamp", scionServerLogPrefix)
			rxt = time.Now().UTC()
		}

		pkt.Bytes = pkt.Bytes[:n]
		err = pkt.Decode()
		if err != nil {
			log.Printf("%s Failed to decode packet: %v", err, scionServerLogPrefix)
			continue
		}

		var ok bool
		udppkt, ok = pkt.Payload.(snet.UDPPayload)
		if !ok {
			log.Printf("%s Packet payload is not a UDP packet", scionServerLogPrefix)
			continue
		}

		if int(udppkt.DstPort) != localHostPort {
			log.Printf("%s Packet destination port does not match local port", scionServerLogPrefix)
			continue
		}

		log.Printf("%s Received payload at %v via %v with flags = %v:", scionServerLogPrefix, rxt, lastHop, flags)
		fmt.Printf("%s", hex.Dump(udppkt.Payload))

		var ntpreq ntp.Packet
		err = ntp.DecodePacket(udppkt.Payload, &ntpreq)
		if err != nil {
			log.Printf("%s Failed to decode packet payload: %v", scionServerLogPrefix, err)
			continue
		}

		li := ntpreq.LeapIndicator()
		if li != ntp.LeapIndicatorNoWarning && li != ntp.LeapIndicatorUnknown {
			log.Printf("%s Unexpected NTP request packet: LI = %v, dropping packet",
				scionServerLogPrefix, li)
			continue
		}
		vn := ntpreq.Version()
		if vn < ntp.VersionMin || ntp.VersionMax < vn {
			log.Printf("%s Unexpected NTP request packet: VN = %v, dropping packet",
				scionServerLogPrefix, vn)
			continue
		}
		mode := ntpreq.Mode()
		if vn == 1 && mode != ntp.ModeReserved0 ||
			vn != 1 && mode != ntp.ModeClient {
			log.Printf("%s Unexpected NTP request packet: Mode = %v, dropping packet",
				scionServerLogPrefix, mode)
			continue
		}
		if vn == 1 && udppkt.SrcPort == ntp.ServerPort {
			log.Printf("%s Unexpected NTP request packet: VN = %v, SrcPort = %v, dropping packet",
				scionServerLogPrefix, vn, udppkt.SrcPort)
			continue
		}

		now := time.Now().UTC()

		ntpresp := ntp.Packet{}
		ntpresp.SetVersion(ntp.VersionMax)
		ntpresp.SetMode(ntp.ModeServer)
		ntpresp.Stratum = 1
		ntpresp.Poll = ntpreq.Poll
		ntpresp.Precision = -32
		ntpresp.RootDispersion = ntp.Time32{ Seconds: 0, Fraction: 10, }
		ntpresp.ReferenceID = ntp.ServerRefID

		ntpresp.ReferenceTime = ntp.Time64FromTime(now)
		ntpresp.OriginTime = ntpreq.TransmitTime
		ntpresp.ReceiveTime = ntp.Time64FromTime(rxt)
		ntpresp.TransmitTime = ntp.Time64FromTime(now)

		ntp.EncodePacket(&ntpresp, &udppkt.Payload)
		udppkt.DstPort, udppkt.SrcPort = udppkt.SrcPort, udppkt.DstPort

		pkt.Destination, pkt.Source = pkt.Source, pkt.Destination
		err = pkt.Path.Reverse()
		if err != nil {
			log.Printf("%s Failed to reverse path: %v", scionServerLogPrefix, err)
			continue
		}
		pkt.Payload = &udppkt
		err = pkt.Serialize()
		if err != nil {
			log.Printf("%s Failed to serialize packet: %v", scionServerLogPrefix, err)
			continue
		}

		n, err = conn.WriteTo(pkt.Bytes, lastHop)
		if err != nil {
			log.Printf("%s Failed to write packet: %v", scionServerLogPrefix, err)
			continue
		}
		if n != len(pkt.Bytes) {
			log.Printf("%s Failed to write entire packet: %v/%v", scionServerLogPrefix, n, len(pkt.Bytes))
			continue
		}
	}
}
