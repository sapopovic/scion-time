package core

import (
	"unsafe"

	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/sys/unix"

	"github.com/facebook/time/ntp/protocol/ntp"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"
)

const scionServerLogPrefix = "[core/server_scion]"

func StartSCIONServer(localIA addr.IA, localHost *net.UDPAddr) error {
	log.Printf("Listening in %v on %v:%d", localIA, localHost.IP, localHost.Port)

	localHostPort := localHost.Port
	_ = localHostPort
	
	localHost.Port = underlay.EndhostPort

	conn, err := net.ListenUDP("udp", localHost)
	if err != nil {
		log.Fatalf("Failed to listen for packets: %v", err)
	}
	defer conn.Close()

	err = ntp.EnableKernelTimestampsSocket(conn)
	if err != nil {
		log.Fatalf("Failed to enable kernel timestamping for packets: %v", err)
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

		var rxt time.Time
		if oobn != 0 {
			ts := (*unix.Timespec)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
			rxt = time.Unix(ts.Unix())
		} else {
			log.Printf("Failed to receive kernel timestamp")
			rxt = time.Now().UTC()
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

		log.Printf("Received payload at %v via %v with flags = %v: \"%v\":", rxt, lastHop, flags)
		fmt.Printf("%s", hex.Dump(pld.Payload))

		ntpreq, err := ntp.BytesToPacket(pld.Payload)
		if err != nil {
			log.Printf("Failed to decode packet payload: %v", err)
			continue
		}

		if !ntpreq.ValidSettingsFormat() {
			log.Printf("Received invalid NTP packet:")
			fmt.Printf("%s", hex.Dump(pld.Payload))
			continue
		}

		log.Printf("Received NTP packet: %+v", ntpreq)

		now := time.Now().UTC()

		ntpresp := &ntp.Packet{
			Stratum: 1,
			Precision: -32,
			RootDelay: 0,
			RootDispersion: 10,
			ReferenceID: binary.BigEndian.Uint32([]byte(fmt.Sprintf("%-4s", "STS0"))),
		}

		ntpresp.Settings = ntpreq.Settings & 0x38
		ntpresp.Poll = ntpreq.Poll

		refTime := time.Unix(now.Unix()/1000*1000, 0)
		ntpresp.RefTimeSec, ntpresp.RefTimeFrac = ntp.Time(refTime)
		ntpresp.OrigTimeSec, ntpresp.OrigTimeFrac = ntpreq.TxTimeSec,ntpreq.TxTimeFrac
		ntpresp.RxTimeSec, ntpresp.RxTimeFrac = ntp.Time(rxt)
		ntpresp.TxTimeSec, ntpresp.TxTimeFrac = ntp.Time(now)

		resppld, err := ntpresp.Bytes()
		if err != nil {
			log.Printf("Failed to encode %+v: %v", ntpresp, err)
			continue
		}

		pkt.Destination, pkt.Source = pkt.Source, pkt.Destination
		pkt.Payload = snet.UDPPayload{
			DstPort: pld.SrcPort,
			SrcPort: pld.DstPort,
			Payload: resppld,
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

		_, err = conn.WriteTo(pkt.Bytes, lastHop)
		if err != nil {
			log.Printf("Failed to write packet: %v", err)
			continue
		}
	}
}
