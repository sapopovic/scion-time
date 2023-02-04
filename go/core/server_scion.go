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

	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/slayers"

	"github.com/scionproto/scion/pkg/private/common"
	"github.com/scionproto/scion/private/topology/underlay"

	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/drkeyutil"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/scion"
	"example.com/scion-time/go/net/scion/spao"
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

	var authBuf, authMAC []byte
	if f != nil {
		authBuf = make([]byte, spao.MACBufferSize)
		authMAC = make([]byte, scion.PacketAuthMACLen)
	}
	tsOpt := &slayers.EndToEndOption{}

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

		if int(udpLayer.DstPort) != localHostPort {
			dstAddrPort := netip.AddrPortFrom(dstAddr, udpLayer.DstPort)
			payload := gopacket.Payload(udpLayer.Payload)

			err = buffer.Clear()
			if err != nil {
				panic(err)
			}

			err = payload.SerializeTo(buffer, options)
			if err != nil {
				panic(err)
			}
			buffer.PushLayer(payload.LayerType())

			err = udpLayer.SerializeTo(buffer, options)
			if err != nil {
				panic(err)
			}
			buffer.PushLayer(udpLayer.LayerType())

			if len(oob) != 0 {
				tsOpt.OptType = scion.OptTypeTimestamp
				tsOpt.OptData = oob
				tsOpt.OptAlign[0] = 0
				tsOpt.OptAlign[1] = 0
				tsOpt.OptDataLen = 0
				tsOpt.ActualLength = 0

				if scionLayer.NextHdr != slayers.End2EndClass {
					e2eLayer = slayers.EndToEndExtn{}
					e2eLayer.NextHdr = slayers.L4UDP
					scionLayer.NextHdr = slayers.End2EndClass
				}
				e2eLayer.Options = append(e2eLayer.Options, tsOpt)
			}

			if scionLayer.NextHdr == slayers.End2EndClass {
				err = e2eLayer.SerializeTo(buffer, options)
				if err != nil {
					panic(err)
				}
				buffer.PushLayer(e2eLayer.LayerType())
			}

			err = scionLayer.SerializeTo(buffer, options)
			if err != nil {
				panic(err)
			}
			buffer.PushLayer(scionLayer.LayerType())

			m, err := conn.WriteToUDPAddrPort(buffer.Bytes(), dstAddrPort)
			if err != nil || m != len(buffer.Bytes()) {
				log.Printf("%s Failed to forward packet: %v, %v\n", scionServerLogPrefix, err, m)
				continue
			}
		} else if localHostPort != underlay.EndhostPort {
			var (
				authOpt *slayers.EndToEndOption
				authKey []byte
			)
			authenticated := false

			if f != nil && len(decoded) >= 3 &&
				decoded[len(decoded)-2] == slayers.LayerTypeEndToEndExtn {
				authOpt, err = e2eLayer.FindOption(slayers.OptTypeAuthenticator)
				if err == nil {
					if len(authOpt.OptData) != scion.PacketAuthOptDataLen {
						panic("unexpected authenticator option data")
					}
					authOptData := authOpt.OptData
					spi := uint32(authOptData[3]) |
						uint32(authOptData[2])<<8 |
						uint32(authOptData[1])<<16 |
						uint32(authOptData[0])<<24
					algo := uint8(authOptData[4])
					if spi == scion.PacketAuthSPIClient && algo == scion.PacketAuthAlgorithm {
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
								authKey = key.Key[:]
								_, err = spao.ComputeAuthCMAC(
									spao.MACInput{
										Key:        authKey,
										Header:     slayers.PacketAuthOption{authOpt},
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
								authenticated = subtle.ConstantTimeCompare(authOptData[scion.PacketAuthMetadataLen:], authMAC) != 0
								if !authenticated {
									log.Printf("%s Failed to authenticate packet", scionServerLogPrefix)
									continue
								}
							}
						}
					}
				}
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

			payload := gopacket.Payload(udpLayer.Payload)

			err = buffer.Clear()
			if err != nil {
				panic(err)
			}

			err = payload.SerializeTo(buffer, options)
			if err != nil {
				panic(err)
			}
			buffer.PushLayer(payload.LayerType())

			err = udpLayer.SerializeTo(buffer, options)
			if err != nil {
				panic(err)
			}
			buffer.PushLayer(udpLayer.LayerType())

			if authenticated {
				scion.PreparePacketAuthOpt(authOpt, scion.PacketAuthSPIServer, scion.PacketAuthAlgorithm)
				_, err = spao.ComputeAuthCMAC(
					spao.MACInput{
						Key:        authKey,
						Header:     slayers.PacketAuthOption{authOpt},
						ScionLayer: &scionLayer,
						PldType:    scionLayer.NextHdr,
						Pld:        buffer.Bytes(),
					},
					authBuf,
					authOpt.OptData[scion.PacketAuthMetadataLen:],
				)
				if err != nil {
					panic(err)
				}

				e2eExtn := slayers.EndToEndExtn{}
				e2eExtn.NextHdr = scionLayer.NextHdr
				e2eExtn.Options = []*slayers.EndToEndOption{authOpt}

				err = e2eExtn.SerializeTo(buffer, options)
				if err != nil {
					panic(err)
				}
				buffer.PushLayer(e2eExtn.LayerType())

				scionLayer.NextHdr = slayers.End2EndClass
			}

			err = scionLayer.SerializeTo(buffer, options)
			if err != nil {
				panic(err)
			}
			buffer.PushLayer(scionLayer.LayerType())

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

func newDaemonConnector(ctx context.Context, daemonAddr string) daemon.Connector {
	s := &daemon.Service{
		Address: daemonAddr,
	}
	c, err := s.Connect(ctx)
	if err != nil {
		log.Fatalf("%s Failed to create SCION Daemon connector: %v", scionServerLogPrefix, err)
	}
	return c
}

func StartSCIONServer(localHost *net.UDPAddr, daemonAddr string) {
	log.Printf("%s Listening on %v:%d via SCION", scionServerLogPrefix, localHost.IP, localHost.Port)

	if localHost.Port == underlay.EndhostPort {
		log.Fatalf("%s Invalid listener port: %v", scionServerLogPrefix, localHost.Port)
	}

	localHostPort := localHost.Port
	localHost.Port = underlay.EndhostPort

	if scionServerNumGoroutine == 1 {
		f := drkeyutil.NewFetcher(newDaemonConnector(context.Background(), daemonAddr))
		conn, err := net.ListenUDP("udp", localHost)
		if err != nil {
			log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
		}
		go runSCIONServer(conn, localHostPort, f)
	} else {
		for i := scionServerNumGoroutine; i > 0; i-- {
			f := drkeyutil.NewFetcher(newDaemonConnector(context.Background(), daemonAddr))
			conn, err := reuseport.ListenPacket("udp", localHost.String())
			if err != nil {
				log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
			}
			go runSCIONServer(conn.(*net.UDPConn), localHostPort, f)
		}
	}
}

func StartSCIONDisptacher(localHost *net.UDPAddr) {
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
}
