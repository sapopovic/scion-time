package main

import (
	"context"
	"flag"
	"log"
	"net"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/sciond"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"
)

func sendHello(sciondAddr string, localAddr snet.UDPAddr, remoteAddr snet.UDPAddr) {
	var err error
	ctx := context.Background()

	sdc, err := sciond.NewService(sciondAddr).Connect(ctx)
	if err != nil {
		log.Fatal("Failed to create SCION connector:", err)
	}

	ps, err := sdc.Paths(ctx, remoteAddr.IA, localAddr.IA, sciond.PathReqFlags{Refresh: true})
	if err != nil {
		log.Fatal("Failed to lookup paths: %v:", err)
	}

	if len(ps) == 0 {
		log.Fatal("No paths to %v available", remoteAddr.IA)
	}

	log.Printf("Available paths to %v:\n", remoteAddr.IA)
	for _, p := range ps {
		log.Printf("\t%v\n", p)
	}

	sp := ps[0]

	log.Printf("Selected path to %v:\n", remoteAddr.IA)
	log.Printf("\t%v\n", sp)

	localAddr.Host.Port = underlay.EndhostPort

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
				Payload: []byte("Hello, world!"),
			},
		},
	}

	nextHop := sp.UnderlayNextHop()
	if nextHop == nil && remoteAddr.IA.Equal(localAddr.IA) {
		nextHop = &net.UDPAddr{
			IP: remoteAddr.Host.IP,
			Port: underlay.EndhostPort,
			Zone: remoteAddr.Host.Zone,
		}
	}

	err = pkt.Serialize()
	if err != nil {
		log.Printf("Failed to serialize SCION packet: %v\n", err)
		return
	}

	conn, err := net.DialUDP("udp", localAddr.Host, nextHop)
	if err != nil {
		log.Printf("Failed to dial UDP connection: %v\n", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(pkt.Bytes)
	if err != nil {
		log.Printf("Failed to write packet: %v\n", err)
		return
	}

	pkt.Prepare()
	n, err := conn.Read(pkt.Bytes)
	if err != nil {
		log.Printf("Failed to read packet: %v\n", err)
		return
	}

	pkt.Bytes = pkt.Bytes[:n]
	err = pkt.Decode()
	if err != nil {
		log.Printf("Failed to decode packet: %v\n", err)
		return
	}

	pld, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		log.Printf("Failed to read packet payload\n")
		return
	}
	log.Printf("Received payload: \"%v\"\n", string(pld.Payload))
}

func main() {
	var sciondAddr string
	var localAddr snet.UDPAddr
	var remoteAddr snet.UDPAddr
	flag.StringVar(&sciondAddr, "sciond", "", "sciond address")
	flag.Var(&localAddr, "local", "Local address")
	flag.Var(&remoteAddr, "remote", "Remote address")
	flag.Parse()

	sendHello(sciondAddr, localAddr, remoteAddr)
}
