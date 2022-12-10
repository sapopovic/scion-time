package core

import (
	"log"
	"net"
	"net/netip"

	"github.com/google/gopacket"

	"github.com/libp2p/go-reuseport"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/slayers"

	"github.com/scionproto/scion/pkg/private/common"
	"github.com/scionproto/scion/private/topology/underlay"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const (
	scionServerLogPrefix  = "[core/server_scion]"
	scionServerLogEnabled = true

	scionServerNumGoroutine = 8
)

func runSCIONServer(conn *net.UDPConn, localHostPort int) {
	var err error

	defer conn.Close()
	udp.EnableTimestamping(conn)

	buf := make([]byte, common.SupportedMTU)
	oob := make([]byte, udp.TimestampLen())

	var (
		scionLayer slayers.SCION
		hbhLayer   slayers.HopByHopExtnSkipper
		e2eLayer   slayers.EndToEndExtnSkipper
		udpLayer   slayers.UDP
		scmpLayer  slayers.SCMP
	)
	scionLayer.RecyclePaths()
	err = udpLayer.SetNetworkLayerForChecksum(&scionLayer)
	if err != nil {
		panic(err)
	}
	err = scmpLayer.SetNetworkLayerForChecksum(&scionLayer)
	if err != nil {
		panic(err)
	}
	parser := gopacket.NewDecodingLayerParser(
		slayers.LayerTypeSCION, &scionLayer, &hbhLayer, &e2eLayer, &udpLayer, &scmpLayer,
	)
	parser.IgnoreUnsupported = true
	decoded := make([]gopacket.LayerType, 4)
	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	for {
		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, lastHop, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			log.Printf("%s Failed to receive packet: %v", scionServerLogPrefix, err)
			continue
		}
		if flags != 0 {
			log.Printf("%s Failed to receive packet, flags: %v", scionServerLogPrefix, flags)
			continue
		}
		oob = oob[:oobn]
		rxt, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			rxt = timebase.Now()
			log.Printf("%s Failed to receive packet timestamp: %v", scionServerLogPrefix, err)
		}
		buf = buf[:n]

		err = parser.DecodeLayers(buf, &decoded)
		if err != nil {
			log.Printf("%s Failed to decode packet: %v", scionServerLogPrefix, err)
			continue
		}
		validType := len(decoded) >= 2 &&
			decoded[len(decoded)-1] == slayers.LayerTypeSCIONUDP
		if !validType {
			log.Printf("%s Failed to receive packet: unexpected type or structure", scionServerLogPrefix, err)
			continue
		}

		if int(udpLayer.DstPort) != localHostPort {
			dstAddr, ok := netip.AddrFromSlice(scionLayer.RawDstAddr)
			if !ok {
				panic("unexpected IP address byte slice")
			}
			dstAddrPort := netip.AddrPortFrom(dstAddr, udpLayer.DstPort)
			m, err := conn.WriteToUDPAddrPort(buf, dstAddrPort)
			if err != nil || m != n {
				log.Printf("%s Failed to forward packet: %v, %v\n", scionServerLogPrefix, err, m)
				continue
			}
		} else {
			if len(decoded) != 2 {
				panic("not yet implemented")
			}

			var ntpreq ntp.Packet
			err = ntp.DecodePacket(&ntpreq, udpLayer.Payload)
			if err != nil {
				log.Printf("%s Failed to decode packet payload: %v", scionServerLogPrefix, err)
				continue
			}
			err = ntp.ValidateRequest(&ntpreq, udpLayer.SrcPort)
			if err != nil {
				log.Printf("%s Unexpected request packet: %v", scionServerLogPrefix, err)
				continue
			}

			if scionServerLogEnabled {
				log.Printf("%s Received request at %v: %+v", scionServerLogPrefix, rxt, ntpreq)
			}

			var ntpresp ntp.Packet
			ntp.HandleRequest(&ntpreq, rxt, &ntpresp)

			scionLayer.DstIA, scionLayer.SrcIA = scionLayer.SrcIA, scionLayer.DstIA
			scionLayer.DstAddrType, scionLayer.SrcAddrType = scionLayer.SrcAddrType, scionLayer.DstAddrType
			scionLayer.RawDstAddr, scionLayer.RawSrcAddr = scionLayer.RawSrcAddr, scionLayer.RawDstAddr
			scionLayer.Path, err = scionLayer.Path.Reverse()
			if err != nil {
				panic(err)
			}

			udpLayer.DstPort, udpLayer.SrcPort = udpLayer.SrcPort, udpLayer.DstPort
			ntp.EncodePacket(&udpLayer.Payload, &ntpresp)

			err = gopacket.SerializeLayers(buffer, options, &scionLayer, &udpLayer, gopacket.Payload(udpLayer.Payload))
			if err != nil {
				panic(err)
			}

			n, err = conn.WriteToUDPAddrPort(buffer.Bytes(), lastHop)
			if err != nil {
				log.Printf("%s Failed to send packet: %v", scionServerLogPrefix, err)
				continue
			}
			if n != len(buffer.Bytes()) {
				log.Printf("%s Failed to send entire packet: %v/%v", scionServerLogPrefix, n, len(buffer.Bytes()))
				continue
			}
		}
	}
}

func StartSCIONServer(localIA addr.IA, localHost *net.UDPAddr) error {
	log.Printf("%s Listening in %v on %v:%d via SCION", scionServerLogPrefix, localIA, localHost.IP, localHost.Port)

	localHostPort := localHost.Port
	localHost.Port = underlay.EndhostPort

	if scionServerNumGoroutine == 1 {
		conn, err := net.ListenUDP("udp", localHost)
		if err != nil {
			log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
		}
		go runSCIONServer(conn, localHostPort)
	} else {
		for i := scionServerNumGoroutine; i > 0; i-- {
			conn, err := reuseport.ListenPacket("udp", localHost.String())
			if err != nil {
				log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
			}
			go runSCIONServer(conn.(*net.UDPConn), localHostPort)
		}
	}

	return nil
}
