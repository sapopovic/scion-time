package core

import (
	"log"
	"net"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const ipServerLogPrefix = "[core/server_ip]"

func runIPServer(conn *net.UDPConn) {
	defer conn.Close()
	udp.EnableTimestamping(conn)

	buf := make([]byte, ntp.PacketLen)
	oob := make([]byte, udp.TimestampLen())
	for {
		oob = oob[:cap(oob)]

		n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			log.Printf("%s Failed to read packet: %v", ipServerLogPrefix, err)
			continue
		}
		if flags != 0 {
			log.Printf("%s Failed to read packet, flags: %v", ipServerLogPrefix, flags)
			continue
		}

		oob = oob[:oobn]
		rxt, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			log.Printf("%s Failed to read packet timestamp: %v", ipServerLogPrefix, err)
			rxt = timebase.Now()
		}
		buf = buf[:n]

		var ntpreq ntp.Packet
		err = ntp.DecodePacket(&ntpreq, buf)
		if err != nil {
			log.Printf("%s Failed to decode packet payload: %v", ipServerLogPrefix, err)
			continue
		}

		log.Printf("%s Received request at %v: %+v", ipServerLogPrefix, rxt, ntpreq)

		err = validateRequest(&ntpreq, srcAddr.Port())
		if err != nil {
			log.Printf("%s Unexpected request packet: %v", scionServerLogPrefix, err)
			continue
		}
		var ntpresp ntp.Packet
		handleRequest(&ntpreq, rxt, &ntpresp)

		ntp.EncodePacket(&buf, &ntpresp)

		n, err = conn.WriteToUDPAddrPort(buf, srcAddr)
		if err != nil {
			log.Printf("%s Failed to write packet: %v", ipServerLogPrefix, err)
			continue
		}
		if n != len(buf) {
			log.Printf("%s Failed to write entire packet: %v/%v", ipServerLogPrefix, n, len(buf))
			continue
		}
	}
}

func StartIPServer(localHost *net.UDPAddr) error {
	log.Printf("%s Listening on %v:%d via IP", ipServerLogPrefix, localHost.IP, localHost.Port)

	conn, err := net.ListenUDP("udp", localHost)
	if err != nil {
		log.Printf("%s Failed to listen for packets: %v", ipServerLogPrefix, err)
		return err
	}

	go runIPServer(conn)

	return nil
}
