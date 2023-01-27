package core

import (
	"context"
	"crypto/subtle"
	"log"
	"net"
	"net/netip"
	"time"

	"github.com/google/gopacket"

	"github.com/libp2p/go-reuseport"

	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/slayers"
	"github.com/scionproto/scion/pkg/spao"

	"github.com/scionproto/scion/pkg/private/common"
	"github.com/scionproto/scion/private/topology/underlay"

	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/drkeyutil"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/scion"
	"example.com/scion-time/go/net/udp"
)

const (
	scionServerLogPrefix  = "[core/server_scion]"
	scionServerLogEnabled = true

	scionServerNumGoroutine = 8
)

func runSCIONServer(conn *net.UDPConn, localHostPort int, f *drkeyutil.Fetcher) {
	defer conn.Close()
	_ = udp.EnableTimestamping(conn)

	ctx := context.Background()

	var txId uint32
	buf := make([]byte, common.SupportedMTU)
	oob := make([]byte, udp.TimestampLen())

	var authBuf, authMAC []byte
	if f != nil {
		authBuf = make([]byte, spao.MACBufferSize)
		authMAC = make([]byte, scion.PacketAuthMACLen)
	}

	var (
		scionLayer slayers.SCION
		hbhLayer   slayers.HopByHopExtnSkipper
		e2eLayer   slayers.EndToEndExtn
		udpLayer   slayers.UDP
		scmpLayer  slayers.SCMP
	)
	scionLayer.RecyclePaths()
	udpLayer.SetNetworkLayerForChecksum(&scionLayer)
	scmpLayer.SetNetworkLayerForChecksum(&scionLayer)
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
			log.Printf("%s Failed to read packet: %v", scionServerLogPrefix, err)
			continue
		}
		if flags != 0 {
			log.Printf("%s Failed to read packet, flags: %v", scionServerLogPrefix, flags)
			continue
		}
		oob = oob[:oobn]
		rxt, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			oob = oob[:0]
			rxt = timebase.Now()
			log.Printf("%s Failed to read packet rx timestamp: %v", scionServerLogPrefix, err)
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
			log.Printf("%s Failed to read packet: unexpected type or structure", scionServerLogPrefix)
			continue
		}

		srcAddr, ok := netip.AddrFromSlice(scionLayer.RawSrcAddr)
		if !ok {
			panic("unexpected IP address byte slice")
		}
		dstAddr, ok := netip.AddrFromSlice(scionLayer.RawDstAddr)
		if !ok {
			panic("unexpected IP address byte slice")
		}

		authenticated := false
		if f != nil && len(decoded) >= 3 &&
			decoded[len(decoded)-2] == slayers.LayerTypeEndToEndExtn {
			opt, err := e2eLayer.FindOption(slayers.OptTypeAuthenticator)
			if err == nil {
				optData := opt.OptData
				if len(optData) == scion.PacketAuthOptDataLen {
					spi := uint32(optData[3]) |
						uint32(optData[2])<<8 |
						uint32(optData[1])<<16 |
						uint32(optData[0])<<24
					algo := uint8(optData[4])
					if spi == scion.PacketAuthClientSPI && algo == scion.PacketAuthAlgorithm {
						sv, err := f.FetchSecretValue(ctx, drkey.SecretValueMeta{
							Validity: rxt,
							ProtoId:  scion.DRKeyProtoIdTS,
						})
						if err == nil {
							key, err := drkeyutil.DeriveHostHostKey(sv, drkey.HostHostMeta{
								ProtoId:  scion.DRKeyProtoIdTS,
								Validity: rxt,
								SrcIA:    scionLayer.DstIA,
								DstIA:    scionLayer.SrcIA,
								SrcHost:  dstAddr.String(),
								DstHost:  srcAddr.String(),
							})
							if err == nil {
								_, err = spao.ComputeAuthCMAC(
									spao.MACInput{
										Key:        key.Key[:],
										Header:     slayers.PacketAuthOption{opt},
										ScionLayer: &scionLayer,
										PldType:    slayers.L4UDP,
										Pld:        buf[len(buf)-int(udpLayer.Length):],
									},
									authBuf,
									authMAC,
								)
								if err != nil {
									panic(err)
								}
								if subtle.ConstantTimeCompare(optData[scion.PacketAuthMetadataLen:], authMAC) != 0 {
									authenticated = true
								}
							}
						}
					}
				}
			}
		}

		if int(udpLayer.DstPort) != localHostPort {
			dstAddrPort := netip.AddrPortFrom(dstAddr, udpLayer.DstPort)

			if len(decoded) != 2 {
				panic("not yet implemented")
			}

			if len(oob) != 0 {
				tsOpt := scion.TimestampOption{EndToEndOption: &slayers.EndToEndOption{}}
				tsOpt.OptType = scion.OptTypeTimestamp
				tsOpt.OptData = oob

				e2eExtn := slayers.EndToEndExtn{}
				e2eExtn.NextHdr = scionLayer.NextHdr
				e2eExtn.Options = []*slayers.EndToEndOption{tsOpt.EndToEndOption}

				scionLayer.NextHdr = slayers.End2EndClass
				err = gopacket.SerializeLayers(buffer, options, &scionLayer, &e2eExtn, &udpLayer, gopacket.Payload(udpLayer.Payload))

				n += (int(e2eExtn.ExtLen) + 1) * 4
			} else {
				err = gopacket.SerializeLayers(buffer, options, &scionLayer, &udpLayer, gopacket.Payload(udpLayer.Payload))
			}
			if err != nil {
				panic(err)
			}

			m, err := conn.WriteToUDPAddrPort(buffer.Bytes(), dstAddrPort)
			if err != nil || m != n {
				log.Printf("%s Failed to forward packet: %v, %v\n", scionServerLogPrefix, err, m)
				continue
			}
		} else if localHostPort != underlay.EndhostPort {
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

			clientID := scionLayer.SrcIA.String() + "," + srcAddr.String()

			if scionServerLogEnabled {
				log.Printf("%s Received request at %v from %s, authenticated: %v: %+v",
					scionServerLogPrefix, rxt, clientID, authenticated, ntpreq)
			}

			var txt0 time.Time
			var ntpresp ntp.Packet
			ntp.HandleRequest(clientID, &ntpreq, &rxt, &txt0, &ntpresp)

			scionLayer.DstIA, scionLayer.SrcIA = scionLayer.SrcIA, scionLayer.DstIA
			scionLayer.DstAddrType, scionLayer.SrcAddrType = scionLayer.SrcAddrType, scionLayer.DstAddrType
			scionLayer.RawDstAddr, scionLayer.RawSrcAddr = scionLayer.RawSrcAddr, scionLayer.RawDstAddr
			scionLayer.Path, err = scionLayer.Path.Reverse()
			if err != nil {
				panic(err)
			}
			scionLayer.NextHdr = slayers.L4UDP

			udpLayer.DstPort, udpLayer.SrcPort = udpLayer.SrcPort, udpLayer.DstPort
			ntp.EncodePacket(&udpLayer.Payload, &ntpresp)

			err = gopacket.SerializeLayers(buffer, options, &scionLayer, &udpLayer, gopacket.Payload(udpLayer.Payload))
			if err != nil {
				panic(err)
			}

			n, err = conn.WriteToUDPAddrPort(buffer.Bytes(), lastHop)
			if err != nil {
				log.Printf("%s Failed to write packet: %v", scionServerLogPrefix, err)
				continue
			}
			if n != len(buffer.Bytes()) {
				log.Printf("%s Failed to write entire packet: %v/%v", scionServerLogPrefix, n, len(buffer.Bytes()))
				continue
			}
			txt1, id, err := udp.ReadTXTimestamp(conn)
			if err != nil {
				txt1 = txt0
				log.Printf("%s Failed to read packet tx timestamp: err = %v", scionServerLogPrefix, err)
			} else if id != txId {
				txt1 = txt0
				log.Printf("%s Failed to read packet tx timestamp: id = %v (expected %v)", scionServerLogPrefix, id, txId)
				txId = id + 1
			} else {
				txId++
			}
			ntp.UpdateTXTimestamp(clientID, rxt, &txt1)
		}
	}
}

func StartSCIONServer(localHost *net.UDPAddr, f *drkeyutil.Fetcher) error {
	log.Printf("%s Listening on %v:%d via SCION", scionServerLogPrefix, localHost.IP, localHost.Port)

	if localHost.Port == underlay.EndhostPort {
		log.Fatalf("%s Invalid listener port: %v", scionServerLogPrefix, localHost.Port)
	}

	localHostPort := localHost.Port
	localHost.Port = underlay.EndhostPort

	if scionServerNumGoroutine == 1 {
		conn, err := net.ListenUDP("udp", localHost)
		if err != nil {
			log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
		}
		go runSCIONServer(conn, localHostPort, f)
	} else {
		for i := scionServerNumGoroutine; i > 0; i-- {
			conn, err := reuseport.ListenPacket("udp", localHost.String())
			if err != nil {
				log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
			}
			go runSCIONServer(conn.(*net.UDPConn), localHostPort, f)
		}
	}

	return nil
}

func StartSCIONDisptacher(localHost *net.UDPAddr) error {
	log.Printf("%s Listening on %v:%d via SCION", scionServerLogPrefix, localHost.IP, underlay.EndhostPort)

	if localHost.Port == underlay.EndhostPort {
		log.Fatalf("%s Invalid listener port: %v", scionServerLogPrefix, localHost.Port)
	}

	localHost.Port = underlay.EndhostPort

	conn, err := net.ListenUDP("udp", localHost)
	if err != nil {
		log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
	}
	go runSCIONServer(conn, localHost.Port, nil /* DRKey fetcher */)

	return nil
}
