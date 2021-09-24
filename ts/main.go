package main

import (
	"unsafe"

	"flag"
	"log"
	"net"
	"time"

	"golang.org/x/sys/unix"

	"github.com/facebookincubator/ntp/protocol/ntp"

	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"
)

func runServer(localAddr snet.UDPAddr) {
	var err error

	localAddr.Host.Port = underlay.EndhostPort

	log.Printf("Listening in %v on %v:%d", localAddr.IA, localAddr.Host.IP, localAddr.Host.Port)

	conn, err := net.ListenUDP("udp", localAddr.Host)
	if err != nil {
		log.Fatalf("Failed to listen for packets: %v", err)
	}
	defer conn.Close()

	err = ntp.EnableKernelTimestampsSocket(conn);
	if err != nil {
		log.Fatalf("Failed to enable kernel timestamoing for packets: %v", err)
	}

	for {
		var pkt snet.Packet
		pkt.Prepare()

		oob := make([]byte, ntp.ControlHeaderSizeBytes)

		n, oobn, flags, lastHop, err := conn.ReadMsgUDP(pkt.Bytes, oob)
		if err != nil {
			log.Printf("Failed to read packet: %v", err)
			continue
		}

		var now time.Time
		if oobn != 0 {
			ts := (*unix.Timespec)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
			now = time.Unix(ts.Unix())
			log.Printf("Received kernel timestamp: %v", now)
		} else {
			now = time.Now().UTC()
		}

		pkt.Bytes = pkt.Bytes[:n]
		err = pkt.Decode()
		if err != nil {
			log.Printf("Failed to decode packet: %v", err)
			continue
		}

		pld, ok := pkt.Payload.(snet.UDPPayload)
		if !ok {
			log.Printf("Failed to read packet payload")
			continue
		}

		log.Printf("Received payload at %v with flags = %v: \"%v\"",
			now, flags, string(pld.Payload))

		ntppkt, err := ntp.BytesToPacket(pld.Payload)
		if err != nil {
			log.Printf("Failed to decode packet payload: %v", err)
			continue
		}

		log.Printf("Received NTP packet: %+v", ntppkt)

		pkt.Destination, pkt.Source = pkt.Source, pkt.Destination
		pkt.Payload = snet.UDPPayload{
			DstPort: pld.SrcPort,
			SrcPort: pld.DstPort,
			Payload: []byte("!DLROW ,OLLEh"),
		}
		if err := pkt.Path.Reverse(); err != nil {
			log.Printf("Failed to reverse path: %v", err)
			continue
		}

		err = pkt.Serialize()
		if err != nil {
			log.Printf("Failed to serialize packet: %v", err)
			continue
		}

		_, err = conn.WriteTo(pkt.Bytes, lastHop);
		if err != nil {
			log.Printf("Failed to write packet: %v", err)
			continue
		}
	}
}

func main() {
	var localAddr snet.UDPAddr
	flag.Var(&localAddr, "local", "Local address")
	flag.Parse()

	runServer(localAddr)
}
