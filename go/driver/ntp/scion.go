package ntp

import (
	"context"
	"log"
	"net"
	"net/netip"
	"time"

	"github.com/google/gopacket"

	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/slayers"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/spao"

	"github.com/scionproto/scion/pkg/private/common"
	"github.com/scionproto/scion/private/topology/underlay"

	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/drkeyutil"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/scion"
	"example.com/scion-time/go/net/udp"
)

type SCIONClient struct {
	InterleavedMode bool
	DRKeyFetcher    *drkeyutil.Fetcher
	auth            struct {
		opt slayers.PacketAuthOption
		buf []byte
		key []byte
	}
	prev struct {
		reference string
		cTxTime   ntp.Time64
		cRxTime   ntp.Time64
		sRxTime   ntp.Time64
	}
}

var defaultSCIONClient = &SCIONClient{}

func compareIPs(x, y []byte) int {
	addrX, okX := netip.AddrFromSlice(x)
	addrY, okY := netip.AddrFromSlice(y)
	if !okX || !okY {
		panic("unexpected IP address byte slice")
	}
	if addrX.Is4In6() {
		addrX = netip.AddrFrom4(addrX.As4())
	}
	if addrY.Is4In6() {
		addrY = netip.AddrFrom4(addrY.As4())
	}
	return addrX.Compare(addrY)
}

func (c *SCIONClient) MeasureClockOffsetSCION(ctx context.Context, localAddr, remoteAddr udp.UDPAddr,
	path snet.Path) (offset time.Duration, weight float64, err error) {
	if c.DRKeyFetcher != nil && c.auth.opt.EndToEndOption == nil {
		c.auth.opt.EndToEndOption = &slayers.EndToEndOption{}
		c.auth.opt.OptData = make([]byte, scion.PacketAuthOptDataLen)
		c.auth.buf = make([]byte, spao.MACBufferSize)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localAddr.Host.IP})
	if err != nil {
		return offset, weight, err
	}
	defer conn.Close()
	deadline, deadlineIsSet := ctx.Deadline()
	if deadlineIsSet {
		err = conn.SetDeadline(deadline)
		if err != nil {
			return offset, weight, err
		}
	}
	_ = udp.EnableTimestamping(conn)

	localPort := conn.LocalAddr().(*net.UDPAddr).Port

	nextHop := path.UnderlayNextHop().AddrPort()
	nextHopAddr := nextHop.Addr()
	if nextHopAddr.Is4In6() {
		nextHop = netip.AddrPortFrom(
			netip.AddrFrom4(nextHopAddr.As4()),
			nextHop.Port())
	}
	if nextHop == (netip.AddrPort{}) && remoteAddr.IA.Equal(localAddr.IA) {
		nextHop = netip.AddrPortFrom(
			netip.AddrFrom4(remoteAddr.Host.AddrPort().Addr().As4()),
			underlay.EndhostPort)
	}

	srcAddr := &net.IPAddr{IP: localAddr.Host.IP}
	dstAddr := &net.IPAddr{IP: remoteAddr.Host.IP}

	buf := make([]byte, common.SupportedMTU)

	reference := remoteAddr.IA.String() + "," + remoteAddr.Host.String()
	cTxTime0 := timebase.Now()

	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	if c.InterleavedMode && reference == c.prev.reference &&
		cTxTime0.Sub(ntp.TimeFromTime64(c.prev.cTxTime)) <= time.Second {
		ntpreq.OriginTime = c.prev.sRxTime
		ntpreq.ReceiveTime = c.prev.cRxTime
		ntpreq.TransmitTime = c.prev.cTxTime
	} else {
		ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime0)
	}
	ntp.EncodePacket(&buf, &ntpreq)

	var scionLayer slayers.SCION
	scionLayer.SrcIA = localAddr.IA
	err = scionLayer.SetSrcAddr(srcAddr)
	if err != nil {
		panic(err)
	}
	scionLayer.DstIA = remoteAddr.IA
	err = scionLayer.SetDstAddr(dstAddr)
	if err != nil {
		panic(err)
	}
	err = path.Dataplane().SetPath(&scionLayer)
	if err != nil {
		panic(err)
	}
	scionLayer.NextHdr = slayers.L4UDP

	var udpLayer slayers.UDP
	udpLayer.SrcPort = uint16(localPort)
	udpLayer.DstPort = uint16(remoteAddr.Host.Port)
	udpLayer.SetNetworkLayerForChecksum(&scionLayer)

	payload := gopacket.Payload(buf)

	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
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

	if c.DRKeyFetcher != nil {
		key, err := c.DRKeyFetcher.FetchHostHostKey(ctx, drkey.HostHostMeta{
			ProtoId:  scion.DRKeyProtoIdTS,
			Validity: cTxTime0,
			SrcIA:    remoteAddr.IA,
			DstIA:    localAddr.IA,
			SrcHost:  remoteAddr.Host.IP.String(),
			DstHost:  localAddr.Host.IP.String(),
		})
		if err == nil {
			c.auth.key = key.Key[:]

			spi := scion.PacketAuthClientSPI
			algo := scion.PacketAuthAlgorithm

			optData := c.auth.opt.OptData[:cap(c.auth.opt.OptData)]
			optData[0] = byte(spi >> 24)
			optData[1] = byte(spi >> 16)
			optData[2] = byte(spi >> 8)
			optData[3] = byte(spi)
			optData[4] = byte(algo)
			// TODO: Timestamp and Sequence Number
			// See https://github.com/scionproto/scion/pull/4300
			optData[5], optData[6], optData[7] = 0, 0, 0
			optData[8], optData[9], optData[10], optData[11] = 0, 0, 0, 0
			// Authenticator
			optData[12], optData[13], optData[14], optData[15] = 0, 0, 0, 0
			optData[16], optData[17], optData[18], optData[19] = 0, 0, 0, 0
			optData[20], optData[21], optData[22], optData[23] = 0, 0, 0, 0
			optData[24], optData[25], optData[26], optData[27] = 0, 0, 0, 0

			c.auth.opt.OptType = slayers.OptTypeAuthenticator
			c.auth.opt.OptData = optData
			c.auth.opt.OptAlign = [2]uint8{4, 2}
			c.auth.opt.OptDataLen = 0
			c.auth.opt.ActualLength = 0

			_, err = spao.ComputeAuthCMAC(
				spao.MACInput{
					Key:        c.auth.key,
					Header:     c.auth.opt,
					ScionLayer: &scionLayer,
					PldType:    scionLayer.NextHdr,
					Pld:        buffer.Bytes(),
				},
				c.auth.buf,
				c.auth.opt.Authenticator(),
			)
			if err != nil {
				panic(err)
			}

			e2eExtn := slayers.EndToEndExtn{}
			e2eExtn.NextHdr = scionLayer.NextHdr
			e2eExtn.Options = []*slayers.EndToEndOption{c.auth.opt.EndToEndOption}

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

	n, err := conn.WriteToUDPAddrPort(buffer.Bytes(), nextHop)
	if err != nil {
		return offset, weight, err
	}
	if n != len(buffer.Bytes()) {
		log.Printf("%s Failed to write entire packet: %v/%v", ntpLogPrefix, n, len(buffer.Bytes()))
		return offset, weight, err
	}
	cTxTime1, id, err := udp.ReadTXTimestamp(conn)
	if err != nil || id != 0 {
		cTxTime1 = timebase.Now()
		log.Printf("%s Failed to read packet timestamp: id = %v, err = %v", ntpLogPrefix, id, err)
	}

	oob := make([]byte, udp.TimestampLen())
	for {
		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, lastHop, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to read packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}
		if flags != 0 {
			err = errUnexpectedPacketFlags
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to read packet, flags: %v", ntpLogPrefix, flags)
				continue
			}
			return offset, weight, err
		}
		oob = oob[:oobn]
		cRxTime, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			cRxTime = timebase.Now()
			log.Printf("%s Failed to read packet timestamp: %v", ntpLogPrefix, err)
		}
		buf = buf[:n]

		var (
			hbhLayer  slayers.HopByHopExtnSkipper
			e2eLayer  slayers.EndToEndExtn
			scmpLayer slayers.SCMP
		)
		parser := gopacket.NewDecodingLayerParser(
			slayers.LayerTypeSCION, &scionLayer, &hbhLayer, &e2eLayer, &udpLayer, &scmpLayer,
		)
		parser.IgnoreUnsupported = true
		decoded := make([]gopacket.LayerType, 4)
		err = parser.DecodeLayers(buf, &decoded)
		if err != nil {
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to decode packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}
		validType := len(decoded) >= 2 &&
			decoded[len(decoded)-1] == slayers.LayerTypeSCIONUDP
		if !validType {
			err = errUnexpectedPacket
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to read packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}
		validSrc := scionLayer.SrcIA.Equal(remoteAddr.IA) &&
			compareIPs(scionLayer.RawSrcAddr, remoteAddr.Host.IP) == 0
		validDst := scionLayer.DstIA.Equal(localAddr.IA) &&
			compareIPs(scionLayer.RawDstAddr, localAddr.Host.IP) == 0
		if !validSrc || !validDst {
			err = errUnexpectedPacket
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to read packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}

		if len(decoded) >= 3 &&
			decoded[len(decoded)-2] == slayers.LayerTypeEndToEndExtn {
			tsOpt, err := e2eLayer.FindOption(scion.OptTypeTimestamp)
			if err == nil {
				log.Print("@@@: tsOpt.OptData=", tsOpt.OptData)
				cRxTime0, err := udp.TimestampFromOOBData(tsOpt.OptData)
				if err == nil {
					cRxTime = cRxTime0
				}
			}
			authOpt, err := e2eLayer.FindOption(slayers.OptTypeAuthenticator)
			if err == nil {
				log.Print("@@@: authOpt.OptData=", authOpt.OptData)
			}
		}

		var ntpresp ntp.Packet
		err = ntp.DecodePacket(&ntpresp, udpLayer.Payload)
		if err != nil {
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to read packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}

		interleaved := false
		if c.InterleavedMode && ntpresp.OriginTime == c.prev.cRxTime {
			interleaved = true
		} else if ntpresp.OriginTime != ntpreq.TransmitTime {
			err = errUnexpectedPacket
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to read packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}

		err = ntp.ValidateMetadata(&ntpresp)
		if err != nil {
			return offset, weight, err
		}

		log.Printf("%s Received packet at %v from %v: %+v", ntpLogPrefix, cRxTime, lastHop, ntpresp)

		sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
		sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

		var t0, t1, t2, t3 time.Time
		if interleaved {
			t0 = ntp.TimeFromTime64(c.prev.cTxTime)
			t1 = ntp.TimeFromTime64(c.prev.sRxTime)
			t2 = sTxTime
			t3 = ntp.TimeFromTime64(c.prev.cRxTime)
		} else {
			t0 = cTxTime1
			t1 = sRxTime
			t2 = sTxTime
			t3 = cRxTime
		}

		err = ntp.ValidateTimestamps(t0, t1, t1, t3)
		if err != nil {
			return offset, weight, err
		}

		off := ntp.ClockOffset(t0, t1, t2, t3)
		rtd := ntp.RoundTripDelay(t0, t1, t2, t3)

		log.Printf("%s %s,%s, interleaved mode: %v, clock offset: %fs (%dns), round trip delay: %fs (%dns)",
			ntpLogPrefix, remoteAddr.IA, remoteAddr.Host, interleaved,
			float64(off.Nanoseconds())/float64(time.Second.Nanoseconds()), off.Nanoseconds(),
			float64(rtd.Nanoseconds())/float64(time.Second.Nanoseconds()), rtd.Nanoseconds())

		if c.InterleavedMode {
			c.prev.reference = reference
			c.prev.cTxTime = ntp.Time64FromTime(cTxTime1)
			c.prev.cRxTime = ntp.Time64FromTime(cRxTime)
			c.prev.sRxTime = ntpresp.ReceiveTime
		}

		// offset, weight = off, 1000.0

		offset, weight = filter(reference, t0, t1, t2, t3)
		break
	}

	return offset, weight, nil
}

func MeasureClockOffsetSCION(ctx context.Context, localAddr, remoteAddr udp.UDPAddr,
	path snet.Path) (offset time.Duration, weight float64, err error) {
	return defaultSCIONClient.MeasureClockOffsetSCION(ctx, localAddr, remoteAddr, path)
}
