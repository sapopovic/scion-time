package core

import (
	"unsafe"

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

	sntp "example.com/scion-time/go/protocol/ntp"
)

const scionServerLogPrefix = "[core/server_scion]"

func prepareOOB(b *[]byte) {
	if *b == nil {
		*b = make([]byte, ntp.ControlHeaderSizeBytes)
	}
	*b = (*b)[:cap(*b)]
}

func StartSCIONServer(localIA addr.IA, localHost *net.UDPAddr) error {
	log.Printf("%s Listening in %v on %v:%d via SCION", scionServerLogPrefix, localIA, localHost.IP, localHost.Port)

	localHostPort := localHost.Port
	localHost.Port = underlay.EndhostPort

	conn, err := net.ListenUDP("udp", localHost)
	if err != nil {
		log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
	}
	defer conn.Close()
	err = ntp.EnableKernelTimestampsSocket(conn)
	if err != nil {
		log.Fatalf("%s Failed to enable kernel timestamping for packets: %v", scionServerLogPrefix, err)
	}

	var pkt snet.Packet
	var oob []byte
	for {
		pkt.Prepare()
		prepareOOB(&oob)

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
			log.Printf("%s Failed to receive kernel timestamp", scionServerLogPrefix)
			rxt = time.Now().UTC()
		}

		pkt.Bytes = pkt.Bytes[:n]
		err = pkt.Decode()
		if err != nil {
			log.Printf("%s Failed to decode packet: %v", err, scionServerLogPrefix)
			continue
		}

		reqpld, ok := pkt.Payload.(snet.UDPPayload)
		if !ok {
			log.Printf("%s Packet payload is not a UDP packet", scionServerLogPrefix)
			continue
		}

		if int(reqpld.DstPort) != localHostPort {
			log.Printf("%s Packet destination port does not match local port", scionServerLogPrefix)
			continue
		}

		log.Printf("%s Received payload at %v via %v with flags = %v:", scionServerLogPrefix, rxt, lastHop, flags)
		fmt.Printf("%s", hex.Dump(reqpld.Payload))

		ntpreq, err := ntp.BytesToPacket(reqpld.Payload)
		if err != nil {
			log.Printf("%s Failed to decode packet payload: %v", scionServerLogPrefix, err)
			continue
		}

		if !ntpreq.ValidSettingsFormat() {
			log.Printf("%s Received invalid NTP packet", scionServerLogPrefix)
			continue
		}

		log.Printf("%s Received NTP packet: %+v", scionServerLogPrefix, ntpreq)

		now := time.Now().UTC()

		ntpresp := &ntp.Packet{
			Stratum: 1,
			Precision: -32,
			RootDelay: 0,
			RootDispersion: 10,
			ReferenceID: sntp.ServerRefID,
		}

		ntpresp.Settings = ntpreq.Settings & 0x38
		ntpresp.Poll = ntpreq.Poll

		refTime := time.Unix(now.Unix()/1000*1000, 0)
		ntpresp.RefTimeSec, ntpresp.RefTimeFrac = ntp.Time(refTime)
		ntpresp.OrigTimeSec, ntpresp.OrigTimeFrac = ntpreq.TxTimeSec, ntpreq.TxTimeFrac
		ntpresp.RxTimeSec, ntpresp.RxTimeFrac = ntp.Time(rxt)
		ntpresp.TxTimeSec, ntpresp.TxTimeFrac = ntp.Time(now)

		resppld, err := ntpresp.Bytes()
		if err != nil {
			log.Printf("%s Failed to encode %+v: %v", scionServerLogPrefix, ntpresp, err)
			continue
		}

		pkt.Destination, pkt.Source = pkt.Source, pkt.Destination
		pkt.Payload = snet.UDPPayload{
			DstPort: reqpld.SrcPort,
			SrcPort: reqpld.DstPort,
			Payload: resppld,
		}
		err = pkt.Path.Reverse()
		if err != nil {
			log.Printf("%s Failed to reverse path: %v", scionServerLogPrefix, err)
			continue
		}
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
