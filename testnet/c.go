package main

import (
	"context"
	"flag"
	"log"
	"net"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/sciond"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/sock/reliable"
	"github.com/scionproto/scion/go/lib/sock/reliable/reconnect"
	"github.com/scionproto/scion/go/lib/topology/underlay"
)

func sendHello(sciondAddr, dispatcherSocket string, localAddr snet.UDPAddr, remoteAddr snet.UDPAddr) {
	var err error
	ctx := context.Background()

	sdc, err := sciond.NewService(sciondAddr).Connect(ctx)
	if err != nil {
		log.Fatal("Failed to create SCION connector:", err)
	}
	pds := &snet.DefaultPacketDispatcherService{
		Dispatcher: reconnect.NewDispatcherService(
			reliable.NewDispatcher(dispatcherSocket)),
		SCMPHandler: snet.DefaultSCMPHandler{
			RevocationHandler: sciond.RevHandler{Connector: sdc},
		},
	}

	ps, err := sdc.Paths(ctx, remoteAddr.IA, localAddr.IA, sciond.PathReqFlags{Refresh: true})
	if err != nil {
		log.Fatal("Failed to lookup core paths: %v:", err)
	}

	log.Printf("Available paths to %v:\n", remoteAddr.IA)
	for _, p := range ps {
		log.Printf("\t%v\n", p)
	}

	sp := ps[0]
	log.Printf("Selected path to %v: %v\n", remoteAddr.IA, sp)

	localAddr.Host.Port = 0
	conn, localPort, err := pds.Register(ctx, localAddr.IA, localAddr.Host, addr.SvcNone)
	if err != nil {
		log.Fatal("Failed to register client socket:", err)
	}

	log.Printf("Sending in %v on %v:%d - %v\n", localAddr.IA, localAddr.Host.IP, localPort, addr.SvcNone)

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
				SrcPort: localPort,
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

	err = conn.WriteTo(pkt, nextHop)
	if err != nil {
		log.Printf("Failed to write packet: %v\n", err)
		return
	}

	var lastHop net.UDPAddr
	err = conn.ReadFrom(pkt, &lastHop)
	if err != nil {
		log.Printf("Failed to read packet: %v\n", err)
		return
	}
	pld, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		log.Printf("Failed to read packet payload\n")
		return
	}
	data := string(pld.Payload);
	log.Printf("Received payload: \"%v\"\n", data)
}

func main() {
	var sciondAddr string
	var dispatcherSocket string
	var localAddr snet.UDPAddr
	var remoteAddr snet.UDPAddr
	flag.StringVar(&sciondAddr, "sciond", "", "SCIOND address")
	flag.StringVar(&dispatcherSocket, "dispatcher-socket", "", "dispatcher socket")
	flag.Var(&localAddr, "local", "Local address")
	flag.Var(&remoteAddr, "remote", "Remote address")
	flag.Parse()

	sendHello(sciondAddr, dispatcherSocket, localAddr, remoteAddr)
}
