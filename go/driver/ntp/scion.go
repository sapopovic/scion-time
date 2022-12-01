package ntp

import (
	"context"
	"log"
	"net"
	"net/netip"
	"time"

	"github.com/google/gopacket"

	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/slayers"

	"github.com/scionproto/scion/pkg/private/common"
	"github.com/scionproto/scion/private/topology/underlay"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

func MeasureClockOffsetSCION(ctx context.Context, localAddr, remoteAddr udp.UDPAddr,
	path snet.Path) (offset time.Duration, weight float64, err error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localAddr.Host.IP})
	if err != nil {
		return offset, weight, err
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if ok {
		err = conn.SetDeadline(deadline)
		if err != nil {
			return offset, weight, err
		}
	}
	udp.EnableTimestamping(conn)

	localPort := conn.LocalAddr().(*net.UDPAddr).Port

	nextHop := path.UnderlayNextHop().AddrPort()
	if nextHop.Addr().Is4In6() {
		nextHop = netip.AddrPortFrom(
			netip.AddrFrom4(nextHop.Addr().As4()),
			nextHop.Port())
	}
	if nextHop == (netip.AddrPort{}) && remoteAddr.IA.Equal(localAddr.IA) {
		nextHop = netip.AddrPortFrom(
			remoteAddr.Host.AddrPort().Addr(),
			underlay.EndhostPort)
	}

	buf := make([]byte, common.SupportedMTU)

	cTxTime := timebase.Now()

	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)
	ntp.EncodePacket(&buf, &ntpreq)

	var layers []gopacket.SerializableLayer

	var scionLayer slayers.SCION
	scionLayer.SrcIA = localAddr.IA
	err = scionLayer.SetSrcAddr(&net.IPAddr{IP: localAddr.Host.IP})
	if err != nil {
		panic(err)
	}
	scionLayer.DstIA = remoteAddr.IA
	err = scionLayer.SetDstAddr(&net.IPAddr{IP: remoteAddr.Host.IP})
	if err != nil {
		panic(err)
	}
	err = path.Dataplane().SetPath(&scionLayer)
	if err != nil {
		panic(err)
	}
	scionLayer.NextHdr = slayers.L4UDP
	scionLayer.PayloadLen = uint16(8 /* udpHdrLen */ + len(buf))
	layers = append(layers, &scionLayer)

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

	_, err = conn.WriteToUDPAddrPort(buffer.Bytes(), nextHop)
	if err != nil {
		return offset, weight, err
	}

	buf = buf[:cap(buf)]
	oob := make([]byte, udp.TimestampLen())
	n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
	if err != nil {
		return offset, weight, err
	}
	if flags != 0 {
		return offset, weight, errUnexpectedPacketFlags
	}
	oob = oob[:oobn]
	cRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		cRxTime = timebase.Now()
		log.Printf("%s Failed to receive packet timestamp: %v", ntpLogPrefix, err)
	}
	buf = buf[:n]

	var (
		hbhLayer  slayers.HopByHopExtnSkipper
		e2eLayer  slayers.EndToEndExtnSkipper
		scmpLayer slayers.SCMP
	)
	parser := gopacket.NewDecodingLayerParser(
		slayers.LayerTypeSCION, &scionLayer, &hbhLayer, &e2eLayer, &udpLayer, &scmpLayer,
	)
	parser.IgnoreUnsupported = true
	decoded := make([]gopacket.LayerType, 4)
	err = parser.DecodeLayers(buf, &decoded)
	if err != nil {
		return offset, weight, err
	}
	if len(decoded) < 2 {
		return offset, weight, errUnexpectedPacketStructure
	}
	if decoded[len(decoded)-1] != slayers.LayerTypeSCIONUDP {
		return offset, weight, errUnexpectedPacketStructure
	}
	_ = scionLayer.SrcIA
	_, err = scionLayer.SrcAddr()
	if err != nil {
		return offset, weight, errUnexpectedPacketStructure
	}
	_ = udpLayer.SrcPort
	_ = scionLayer.DstIA
	_, err = scionLayer.DstAddr()
	if err != nil {
		return offset, weight, errUnexpectedPacketStructure
	}
	_ = udpLayer.DstPort
	rpath := snet.RawPath{
		PathType: scionLayer.Path.Type(),
	}
	if l := scionLayer.Path.Len(); l != 0 {
		rpath.Raw = make([]byte, l)
		err := scionLayer.Path.SerializeTo(rpath.Raw)
			if err != nil {
				return offset, weight, errUnexpectedPacketStructure
		}
	}

	var ntpresp ntp.Packet
	err = ntp.DecodePacket(&ntpresp, udpLayer.Payload)
	if err != nil {
		return offset, weight, err
	}

	err = ntp.ValidateResponse(&ntpresp, cTxTime)
	if err != nil {
		return offset, weight, err
	}

	log.Printf("%s Received packet at %v from %v: %+v", ntpLogPrefix, cRxTime, srcAddr, ntpresp)

	sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
	sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

	off := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	rtd := ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime)

	log.Printf("%s %s,%s clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		ntpLogPrefix, remoteAddr.IA, remoteAddr.Host,
		float64(off.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(off.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	// offset, weight = off, 1000.0

	reference := remoteAddr.IA.String() + "," + remoteAddr.Host.String()
	offset, weight = filter(reference, cTxTime, sRxTime, sTxTime, cRxTime)

	return offset, weight, nil
}
