package core

import (
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"time"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const ipServerLogPrefix = "[core/server_ip]"

func StartIPServer(localHost *net.UDPAddr) error {
	log.Printf("%s Listening on %v:%d via IP", scionServerLogPrefix, localHost.IP, localHost.Port)

	conn, err := net.ListenUDP("udp", localHost)
	if err != nil {
		log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
	}
	defer conn.Close()
	udp.EnableTimestamping(conn)

	buf := make([]byte, ntp.PacketLen)
	oob := make([]byte, udp.TimestampOutOfBandDataLen())
	for {
		oob = oob[:cap(oob)]

		n, oobn, flags, lastHop, err := conn.ReadMsgUDP(buf, oob)
		if err != nil {
			log.Printf("%s Failed to read packet: %v", scionServerLogPrefix, err)
			continue
		}

		oob = oob[:oobn]
		rxt, err := udp.TimeFromOutOfBandData(oob)
		if err != nil {
			log.Printf("%s Failed to read packet timestamp: %v", scionServerLogPrefix, err)
			rxt = time.Now().UTC()
		}

		buf = buf[:n]

		log.Printf("%s Received payload at %v via %v with flags = %v:", scionServerLogPrefix, rxt, lastHop, flags)
		fmt.Printf("%s", hex.Dump(buf))

		var ntpreq ntp.Packet
		err = ntp.DecodePacket(&ntpreq, buf)
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
		if vn == 1 && lastHop.Port == ntp.ServerPort {
			log.Printf("%s Unexpected NTP request packet: VN = %v, SrcPort = %v, dropping packet",
				scionServerLogPrefix, vn, lastHop.Port)
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

		ntp.EncodePacket(&buf, &ntpresp)

		n, err = conn.WriteTo(buf, lastHop)
		if err != nil {
			log.Printf("%s Failed to write packet: %v", scionServerLogPrefix, err)
			continue
		}
		if n != len(buf) {
			log.Printf("%s Failed to write entire packet: %v/%v", scionServerLogPrefix, n, len(buf))
			continue
		}
	}
}
