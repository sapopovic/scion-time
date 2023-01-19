package core

import (
	"log"
	"net"
	"time"

	"github.com/libp2p/go-reuseport"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const (
	ipServerLogPrefix  = "[core/server_ip]"
	ipServerLogEnabled = false

	ipServerNumGoroutine = 8
)

func runIPServer(conn *net.UDPConn) {
	defer conn.Close()
	_ = udp.EnableTimestamping(conn)

	var txId uint32
	buf := make([]byte, ntp.PacketLen)
	oob := make([]byte, udp.TimestampLen())
	for {
		buf = buf[:cap(buf)]
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
			oob = oob[:0]
			rxt = timebase.Now()
			log.Printf("%s Failed to read packet rx timestamp: %v", ipServerLogPrefix, err)
		}
		buf = buf[:n]

		var ntpreq ntp.Packet
		err = ntp.DecodePacket(&ntpreq, buf)
		if err != nil {
			log.Printf("%s Failed to decode packet payload: %v", ipServerLogPrefix, err)
			continue
		}

		err = ntp.ValidateRequest(&ntpreq, srcAddr.Port())
		if err != nil {
			log.Printf("%s Unexpected request packet: %v", ipServerLogPrefix, err)
			continue
		}

		clientID := srcAddr.Addr().String()

		if ipServerLogEnabled {
			log.Printf("%s Received request at %v from %s: %+v", ipServerLogPrefix, rxt, clientID, ntpreq)
		}

		var txt0 time.Time
		var ntpresp ntp.Packet
		ntp.HandleRequest(clientID, &ntpreq, &rxt, &txt0, &ntpresp)

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
		txt1, id, err := udp.ReadTXTimestamp(conn)
		if err != nil {
			log.Printf("%s Failed to read packet tx timestamp: err = %v", ipServerLogPrefix, err)
		} else if id != txId {
			log.Printf("%s Failed to read packet tx timestamp: id = %v (expected %v)", ipServerLogPrefix, id, txId)
			txId = id + 1
		} else {
			ntp.UpdateTXTimestamp(clientID, rxt, &txt1)
			txId++
		}
	}
}

func StartIPServer(localHost *net.UDPAddr) error {
	log.Printf("%s Listening on %v:%d via IP", ipServerLogPrefix, localHost.IP, localHost.Port)

	if ipServerNumGoroutine == 1 {
		conn, err := net.ListenUDP("udp", localHost)
		if err != nil {
			log.Fatalf("%s Failed to listen for packets: %v", ipServerLogPrefix, err)
		}
		go runIPServer(conn)
	} else {
		for i := ipServerNumGoroutine; i > 0; i-- {
			conn, err := reuseport.ListenPacket("udp", localHost.String())
			if err != nil {
				log.Fatalf("%s Failed to listen for packets: %v", ipServerLogPrefix, err)
			}
			go runIPServer(conn.(*net.UDPConn))
		}
	}

	return nil
}
