package core

import (
	"unsafe"

	"encoding/hex"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/sys/unix"

	fbntp "github.com/facebook/time/ntp/protocol/ntp"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	"example.com/scion-time/go/protocol/ntp"
)

const scionServerLogPrefix = "[core/server_scion]"

func prepareOOB(b *[]byte) {
	if *b == nil {
		*b = make([]byte, fbntp.ControlHeaderSizeBytes)
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
	err = fbntp.EnableKernelTimestampsSocket(conn)
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

		ntpreq0, err := fbntp.BytesToPacket(reqpld.Payload)
		if err != nil {
			log.Printf("%s Failed to decode packet payload: %v", scionServerLogPrefix, err)
			continue
		}

		var ntpreq ntp.Packet
		err = ntp.DecodePacket(reqpld.Payload, &ntpreq)
		if err != nil {
			log.Printf("%s Failed to decode packet payload: %v", scionServerLogPrefix, err)
			continue
		}

		if ntpreq.LIVNMode != ntpreq0.Settings ||
			ntpreq.Stratum != ntpreq0.Stratum ||
			ntpreq.Poll != ntpreq0.Poll ||
			ntpreq.Precision != ntpreq0.Precision ||
			ntpreq.RootDelay.Seconds != uint16(ntpreq0.RootDelay >> 16) ||
			ntpreq.RootDelay.Fraction != uint16(ntpreq0.RootDelay) ||
			ntpreq.RootDispersion.Seconds != uint16(ntpreq0.RootDispersion >> 16) ||
			ntpreq.RootDispersion.Fraction != uint16(ntpreq0.RootDispersion) ||
			ntpreq.ReferenceID != ntpreq0.ReferenceID ||
			ntpreq.ReferenceTime.Seconds != ntpreq0.RefTimeSec ||
			ntpreq.ReferenceTime.Fraction != ntpreq0.RefTimeFrac ||
			ntpreq.OriginTime.Seconds != ntpreq0.OrigTimeSec ||
			ntpreq.OriginTime.Fraction != ntpreq0.OrigTimeFrac ||
			ntpreq.ReceiveTime.Seconds != ntpreq0.RxTimeSec ||
			ntpreq.ReceiveTime.Fraction != ntpreq0.RxTimeFrac ||
			ntpreq.TransmitTime.Seconds != ntpreq0.TxTimeSec ||
			ntpreq.TransmitTime.Fraction != ntpreq0.TxTimeFrac {
			panic("NTP packet decoder error")
		}
		log.Printf("%s NTP packet decoder check passed", scionServerLogPrefix)	

		log.Printf("%s Received NTP packet: %+v", scionServerLogPrefix, ntpreq)

		now := time.Now().UTC()

		ntpresp := &fbntp.Packet{
			Stratum: 1,
			Precision: -32,
			RootDelay: 0,
			RootDispersion: 10,
			ReferenceID: ntp.ServerRefID,
		}

		ntpresp.Settings = ntpreq0.Settings & 0x38
		ntpresp.Poll = ntpreq0.Poll

		refTime := time.Unix(now.Unix()/1000*1000, 0)
		ntpresp.RefTimeSec, ntpresp.RefTimeFrac = fbntp.Time(refTime)
		ntpresp.OrigTimeSec, ntpresp.OrigTimeFrac = ntpreq0.TxTimeSec, ntpreq0.TxTimeFrac
		ntpresp.RxTimeSec, ntpresp.RxTimeFrac = fbntp.Time(rxt)
		ntpresp.TxTimeSec, ntpresp.TxTimeFrac = fbntp.Time(now)

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
