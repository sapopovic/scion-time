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

	var authKey, authBuf, authMAC, authOptData []byte
	var authOpt slayers.PacketAuthOption
	if f != nil {
		authBuf = make([]byte, spao.MACBufferSize)
		authMAC = make([]byte, scion.PacketAuthMACLen)
		authOpt.EndToEndOption = &slayers.EndToEndOption{}
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
		authKey = nil
		authOptData = nil

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
				authOptData = opt.OptData
				if len(authOptData) == scion.PacketAuthOptDataLen {
					spi := uint32(authOptData[3]) |
						uint32(authOptData[2])<<8 |
						uint32(authOptData[1])<<16 |
						uint32(authOptData[0])<<24
					algo := uint8(authOptData[4])
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
								authKey = key.Key[:]
								_, err = spao.ComputeAuthCMAC(
									spao.MACInput{
										Key:        authKey,
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
								if subtle.ConstantTimeCompare(authOptData[scion.PacketAuthMetadataLen:], authMAC) != 0 {
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

			payload := gopacket.Payload(udpLayer.Payload)

			buffer.Clear()

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
				tsOpt := scion.TimestampOption{EndToEndOption: &slayers.EndToEndOption{}}
				tsOpt.OptType = scion.OptTypeTimestamp
				tsOpt.OptData = oob

				e2eExtn := slayers.EndToEndExtn{}
				e2eExtn.NextHdr = scionLayer.NextHdr
				e2eExtn.Options = []*slayers.EndToEndOption{tsOpt.EndToEndOption}

				err = e2eExtn.SerializeTo(buffer, options)
				if err != nil {
					panic(err)
				}
				buffer.PushLayer(e2eExtn.LayerType())

				n += (int(e2eExtn.ExtLen) + 1) * 4

				scionLayer.NextHdr = slayers.End2EndClass
			}

			err = scionLayer.SerializeTo(buffer, options)
			if err != nil {
				panic(err)
			}
			buffer.PushLayer(scionLayer.LayerType())

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

			payload := gopacket.Payload(udpLayer.Payload)

			buffer.Clear()

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

			if authKey != nil {
				spi := scion.PacketAuthServerSPI
				algo := scion.PacketAuthAlgorithm

				authOptData = authOptData[:scion.PacketAuthOptDataLen]
				authOptData[0] = byte(spi >> 24)
				authOptData[1] = byte(spi >> 16)
				authOptData[2] = byte(spi >> 8)
				authOptData[3] = byte(spi)
				authOptData[4] = byte(algo)
				// TODO: Timestamp and Sequence Number
				// See https://github.com/scionproto/scion/pull/4300
				authOptData[5], authOptData[6], authOptData[7] = 0, 0, 0
				authOptData[8], authOptData[9], authOptData[10], authOptData[11] = 0, 0, 0, 0
				// Authenticator
				authOptData[12], authOptData[13], authOptData[14], authOptData[15] = 0, 0, 0, 0
				authOptData[16], authOptData[17], authOptData[18], authOptData[19] = 0, 0, 0, 0
				authOptData[20], authOptData[21], authOptData[22], authOptData[23] = 0, 0, 0, 0
				authOptData[24], authOptData[25], authOptData[26], authOptData[27] = 0, 0, 0, 0

				authOpt.OptType = slayers.OptTypeAuthenticator
				authOpt.OptData = authOptData
				authOpt.OptAlign = [2]uint8{4, 2}
				authOpt.OptDataLen = 0
				authOpt.ActualLength = 0

				_, err = spao.ComputeAuthCMAC(
					spao.MACInput{
						Key:        authKey,
						Header:     authOpt,
						ScionLayer: &scionLayer,
						PldType:    scionLayer.NextHdr,
						Pld:        buffer.Bytes(),
					},
					authBuf,
					authOpt.Authenticator(),
				)
				if err != nil {
					panic(err)
				}

				e2eExtn := slayers.EndToEndExtn{}
				e2eExtn.NextHdr = scionLayer.NextHdr
				e2eExtn.Options = []*slayers.EndToEndOption{authOpt.EndToEndOption}

if false /* @@@ */ {
				err = e2eExtn.SerializeTo(buffer, options)
				if err != nil {
					panic(err)
				}
				buffer.PushLayer(e2eExtn.LayerType())

				scionLayer.NextHdr = slayers.End2EndClass
}
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

func StartSCIONServer(localHost *net.UDPAddr, f *drkeyutil.Fetcher) {
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
