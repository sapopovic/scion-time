package main

import (
	"unsafe"

	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"net"
	"time"

	"golang.org/x/sys/unix"

	"github.com/facebookincubator/ntp/protocol/ntp"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/sciond"
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

		_, err = conn.WriteTo(pkt.Bytes, lastHop);
		if err != nil {
			log.Printf("Failed to write packet: %v", err)
			continue
		}
	}
}

func runClient(sciondAddr string, localAddr snet.UDPAddr, remoteAddr snet.UDPAddr) {
	var err error
	ctx := context.Background()

	sdc, err := sciond.NewService(sciondAddr).Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to create SCION connector: %v", err)
	}

	ps, err := sdc.Paths(ctx, remoteAddr.IA, localAddr.IA, sciond.PathReqFlags{Refresh: true})
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

	buf := new(bytes.Buffer)
	sec, frac := ntp.Time(time.Now().UTC())
	request := &ntp.Packet{
		Settings:   0x1B,
		TxTimeSec:  sec,
		TxTimeFrac: frac,
	}
	err = binary.Write(buf, binary.BigEndian, request);
	if err != nil {
		log.Fatalf("Failed to send NTP packet, %v", err)
	}

	pkt := &snet.Packet{
		PacketInfo: snet.PacketInfo{
			Source: snet.SCIONAddress{
				IA: localAddr.IA,
				Host: addr.HostFromIP(localAddr.Host.IP),
			},
			Destination: snet.SCIONAddress{
				IA: remoteAddr.IA,
				Host: addr.HostFromIP(remoteAddr.Host.IP),
			},
			Path: sp.Path(),
			Payload: snet.UDPPayload{
				SrcPort: uint16(localAddr.Host.Port),
				DstPort: uint16(remoteAddr.Host.Port),
				Payload: buf.Bytes(),
			},
		},
	}

	err = pkt.Serialize()
	if err != nil {
		log.Printf("Failed to serialize packet: %v", err)
		return
	}

	nextHop := sp.UnderlayNextHop()
	if nextHop == nil && remoteAddr.IA.Equal(localAddr.IA) {
		nextHop = &net.UDPAddr{
			IP: remoteAddr.Host.IP,
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

	err = ntp.EnableKernelTimestampsSocket(conn);
	if err != nil {
		log.Fatalf("Failed to enable kernel timestamping for packets: %v", err)
	}

	_, err = conn.Write(pkt.Bytes)
	if err != nil {
		log.Printf("Failed to write packet: %v", err)
		return
	}

	pkt.Prepare()
	oob := make([]byte, ntp.ControlHeaderSizeBytes)

	n, oobn, flags, lastHop, err := conn.ReadMsgUDP(pkt.Bytes, oob)
	if err != nil {
		log.Printf("Failed to read packet: %v", err)
		return
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
		return
	}

	pld, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		log.Printf("Failed to read packet payload")
		return
	}

	log.Printf("Received payload at %v via %v with flags = %v: \"%v\":", rxt, lastHop, flags)
	fmt.Printf("%s", hex.Dump(pld.Payload))

	ntpreq, err := ntp.BytesToPacket(pld.Payload)
	if err != nil {
		log.Printf("Failed to decode packet payload: %v", err)
		return
	}

	log.Printf("Received NTP packet: %+v", ntpreq)
}

func main() {
	var sciondAddr string
	var localAddr snet.UDPAddr
	var remoteAddr snet.UDPAddr

	serverFlags := flag.NewFlagSet("server", flag.ExitOnError)
	relayFlags := flag.NewFlagSet("relay", flag.ExitOnError)
	clientFlags := flag.NewFlagSet("client", flag.ExitOnError)

	serverFlags.Var(&localAddr, "local", "Local address")

	clientFlags.StringVar(&sciondAddr, "sciond", "", "sciond address")
	clientFlags.Var(&localAddr, "local", "Local address")
	clientFlags.Var(&remoteAddr, "remote", "Remote address")

	if len(os.Args) < 2 {
		fmt.Println("<usage>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		serverFlags.Parse(os.Args[2:])
		runServer(localAddr)
	case "relay":
		relayFlags.Parse(os.Args[2:])
	case "client":
		clientFlags.Parse(os.Args[2:])
		log.Print("sciondAddr:", sciondAddr)
		log.Print("localAddr:", localAddr)
		log.Print("remoteAddr:", remoteAddr)
		runClient(sciondAddr, localAddr, remoteAddr)
	default:
		fmt.Println("<usage>")
		os.Exit(1)
	}
}
