package ntp

import (
	"context"
	"log"
	"math"
	"net"
	"net/netip"
	"time"

	"github.com/google/gopacket"

	"github.com/scionproto/scion/pkg/slayers"
	"github.com/scionproto/scion/pkg/snet"

	"github.com/scionproto/scion/pkg/private/common"
	"github.com/scionproto/scion/private/topology/underlay"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/scion"
	"example.com/scion-time/go/net/udp"
)

const (
	authOptDataLen = 12 /* len(metadata) */ + 16 /* len(MAC) */
	udpHdrLen      = 8
)

type SCIONClient struct {
	Authenticated bool
	Interleaved   bool
	authOpt       slayers.PacketAuthOption
	prev          struct {
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
	if c.Authenticated {
		if c.authOpt.EndToEndOption == nil {
			c.authOpt.EndToEndOption = &slayers.EndToEndOption{}
			c.authOpt.OptData = make([]byte, authOptDataLen)
		}
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
	if c.Interleaved && reference == c.prev.reference &&
		cTxTime0.Sub(ntp.TimeFromTime64(c.prev.cTxTime)) <= time.Second {
		ntpreq.OriginTime = c.prev.sRxTime
		ntpreq.ReceiveTime = c.prev.cRxTime
		ntpreq.TransmitTime = c.prev.cTxTime
	} else {
		ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime0)
	}
	ntp.EncodePacket(&buf, &ntpreq)

	var layers []gopacket.SerializableLayer

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
	if len(buf) > math.MaxUint16-udpHdrLen {
		panic("payload too large")
	}
	scionLayer.PayloadLen = uint16(udpHdrLen + len(buf))
	layers = append(layers, &scionLayer)

	if c.Authenticated {
		spi := scion.PacketAuthClientSPI
		algo := scion.PacketAuthAlgorithm
		ts := uint32(0) // @@@
		sn := uint32(0) // @@@

		optData := c.authOpt.OptData[:cap(c.authOpt.OptData)]
		optData[0] = byte(spi >> 24)
		optData[1] = byte(spi >> 16)
		optData[2] = byte(spi >> 8)
		optData[3] = byte(spi)
		optData[4] = byte(algo)
		optData[5] = byte(ts >> 16)
		optData[6] = byte(ts >> 8)
		optData[7] = byte(ts)
		optData[8] = 0
		optData[9] = byte(sn >> 16)
		optData[10] = byte(sn >> 8)
		optData[11] = byte(sn)
		optData[12], optData[13], optData[14], optData[15] = 0, 0, 0, 0
		optData[16], optData[17], optData[18], optData[19] = 0, 0, 0, 0
		optData[20], optData[21], optData[22], optData[23] = 0, 0, 0, 0
		optData[24], optData[25], optData[26], optData[27] = 0, 0, 0, 0

		c.authOpt.OptType = slayers.OptTypeAuthenticator
		c.authOpt.OptData = optData
		c.authOpt.OptAlign = [2]uint8{4, 2}
		c.authOpt.OptDataLen = 0
		c.authOpt.ActualLength = 0

		// @@@
	}

	var udpLayer slayers.UDP
	udpLayer.SrcPort = uint16(localPort)
	udpLayer.DstPort = uint16(remoteAddr.Host.Port)
	err = udpLayer.SetNetworkLayerForChecksum(&scionLayer)
	if err != nil {
		panic(err)
	}
	layers = append(layers, &udpLayer, gopacket.Payload(buf))

	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	err = gopacket.SerializeLayers(buffer, options, layers...)
	if err != nil {
		panic(err)
	}

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
				cRxTime0, err := udp.TimestampFromOOBData(tsOpt.OptData)
				if err == nil {
					cRxTime = cRxTime0
				}
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
		if c.Interleaved && ntpresp.OriginTime == c.prev.cRxTime {
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

		if c.Interleaved {
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
