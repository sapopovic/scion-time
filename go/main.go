package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/config"
	"github.com/scionproto/scion/go/lib/daemon"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	mbgd "example.com/scion-time/go/driver/mbg"
	ntpd "example.com/scion-time/go/driver/ntp"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"

	"example.com/scion-time/go/core"
)

const (
	refClockImpact       = 1.25
	refClockCutoff       = 0
	refClockSyncTimeout  = 5 * time.Second
	refClockSyncInterval = 60 * time.Second
	netClockImpact       = 2.5
	netClockCutoff       = time.Millisecond
	netClockSyncTimeout  = 5 * time.Second
	netClockSyncInterval = 3600 * time.Second
)

type tsConfig struct {
	MBGTimeSources []string `toml:"mbg_time_sources,omitempty"`
	NTPTimeSources []string `toml:"ntp_time_sources,omitempty"`
	SCIONPeers     []string `toml:"scion_peers,omitempty"`
}

type mbgTimeSource string
type ntpTimeSource string

var (
	timeSources []core.TimeSource
	pathInfo    core.PathInfo

	refcc core.ReferenceClockClient
	netcc core.NetworkClockClient
)

func (s mbgTimeSource) MeasureClockOffset() (time.Duration, error) {
	return mbgd.MeasureClockOffset(string(s))
}

func (s ntpTimeSource) MeasureClockOffset() (time.Duration, error) {
	return ntpd.MeasureClockOffset(string(s))
}

func newDaemonConnector(daemonAddr string) daemon.Connector {
	s := &daemon.Service{
		Address: daemonAddr,
	}
	c, err := s.Connect(context.Background())
	if err != nil {
		log.Fatal("Failed to create SCION Daemon connector:", err)
	}
	return c
}

func handlePathInfos(pis <-chan core.PathInfo) {
	for {
		pathInfo = <-pis
	}
}

func measureOffsetToRefClock(tss []core.TimeSource, timeout time.Duration) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return refcc.MeasureClockOffset(ctx, tss)
}

func syncToRefClock(lclk core.LocalClock) {
	for {
		corr, err := measureOffsetToRefClock(timeSources, refClockSyncTimeout)
		if err == nil {
			if corr != 0 {
				lclk.Step(corr)
			}
			return
		}
		lclk.Sleep(time.Second)
	}
}

func runLocalClockSync(lclk core.LocalClock) {
	if refClockImpact <= 1.0 {
		panic("invalid reference clock impact factor")
	}
	if refClockSyncInterval <= 0 {
		panic("invalid reference clock sync interval")
	}
	if refClockSyncTimeout < 0 || refClockSyncTimeout >= refClockSyncInterval/2 {
		panic("invalid reference clock sync timeout")
	}
	maxCorr := refClockImpact * float64(lclk.MaxDrift(refClockSyncInterval))
	for {
		corr, err := measureOffsetToRefClock(timeSources, refClockSyncTimeout)
		if err == nil && core.Abs(corr) > refClockCutoff {
			if float64(core.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(core.Sign(corr)) * maxCorr)
			}
			lclk.Adjust(corr, refClockSyncInterval)
		}
		lclk.Sleep(refClockSyncInterval)
	}
}

func measureOffsetToNetClock(pi core.PathInfo, timeout time.Duration) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return netcc.MeasureClockOffset(ctx, pi)
}

func runGlobalClockSync(lclk core.LocalClock) {
	if netClockImpact <= 1.0 {
		panic("invalid network clock impact factor")
	}
	if netClockImpact-1.0 <= refClockImpact {
		panic("invalid network clock impact factor")
	}
	if netClockSyncInterval < refClockSyncInterval {
		panic("invalid network clock sync interval")
	}
	if netClockSyncTimeout < 0 || netClockSyncTimeout >= netClockSyncInterval/2 {
		panic("invalid network clock sync timeout")
	}
	maxCorr := netClockImpact * float64(lclk.MaxDrift(netClockSyncInterval))
	for {
		corr, err := measureOffsetToNetClock(pathInfo, netClockSyncTimeout)
		if err == nil && core.Abs(corr) > netClockCutoff {
			if float64(core.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(core.Sign(corr)) * maxCorr)
			}
			lclk.Adjust(corr, netClockSyncInterval)
		}
		lclk.Sleep(netClockSyncInterval)
	}
}

func runServer(configFile, daemonAddr string, localAddr snet.UDPAddr) {
	var cfg tsConfig
	err := config.LoadFile(configFile, &cfg)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	for _, s := range cfg.MBGTimeSources {
		log.Print("mbg_time_source: ", s)
		timeSources = append(timeSources, mbgTimeSource(s))
	}
	for _, s := range cfg.NTPTimeSources {
		log.Print("ntp_time_source: ", s)
		timeSources = append(timeSources, ntpTimeSource(s))
	}

	var peerIAs []addr.IA
	var peerHosts []*net.UDPAddr
	for _, s := range cfg.SCIONPeers {
		log.Print("scion_peer: ", s)
		addr, err := snet.ParseUDPAddr(s)
		if err != nil {
			log.Fatalf("Failed to parse peer address \"%s\": %v", s, err)
		}
		peerIAs = append(peerIAs, addr.IA)
		peerHosts = append(peerHosts, addr.Host)
	}

	pathInfos, err := core.StartPather(newDaemonConnector(daemonAddr), peerIAs)
	if err != nil {
		log.Fatal("Failed to start pather:", err)
	}
	go handlePathInfos(pathInfos)

	lclk := &core.SystemClock{}
	syncToRefClock(lclk)

	err = core.StartIPServer(snet.CopyUDPAddr(localAddr.Host))
	if err != nil {
		log.Fatalf("Failed to start IP server: %v", err)
	}
	err = core.StartSCIONServer(localAddr.IA, snet.CopyUDPAddr(localAddr.Host))
	if err != nil {
		log.Fatalf("Failed to start SCION server: %v", err)
	}

	go runLocalClockSync(lclk)
	go runGlobalClockSync(lclk)

	select {}
}

func runClient(daemonAddr string, localAddr snet.UDPAddr, remoteAddr snet.UDPAddr) {
	var err error
	ctx := context.Background()

	dc, err := daemon.NewService(daemonAddr).Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to create SCION daemon connector: %v", err)
	}

	ps, err := dc.Paths(ctx, remoteAddr.IA, localAddr.IA, daemon.PathReqFlags{Refresh: true})
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

	nextHop := sp.UnderlayNextHop()
	if nextHop == nil && remoteAddr.IA.Equal(localAddr.IA) {
		nextHop = &net.UDPAddr{
			IP:   remoteAddr.Host.IP,
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
	udp.EnableTimestamping(conn)

	ntpreq := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)

	clientTxTime := time.Now().UTC()

	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(clientTxTime)
	ntp.EncodePacket(&buf, &ntpreq)

	pkt := &snet.Packet{
		PacketInfo: snet.PacketInfo{
			Source: snet.SCIONAddress{
				IA:   localAddr.IA,
				Host: addr.HostFromIP(localAddr.Host.IP),
			},
			Destination: snet.SCIONAddress{
				IA:   remoteAddr.IA,
				Host: addr.HostFromIP(remoteAddr.Host.IP),
			},
			Path: sp.Path(),
			Payload: snet.UDPPayload{
				SrcPort: uint16(localAddr.Host.Port),
				DstPort: uint16(remoteAddr.Host.Port),
				Payload: buf,
			},
		},
	}

	err = pkt.Serialize()
	if err != nil {
		log.Printf("Failed to serialize packet: %v", err)
		return
	}

	_, err = conn.Write(pkt.Bytes)
	if err != nil {
		log.Printf("Failed to write packet: %v", err)
		return
	}

	pkt.Prepare()
	oob := make([]byte, udp.TimestampLen())

	n, oobn, flags, lastHop, err := conn.ReadMsgUDP(pkt.Bytes, oob)
	if err != nil {
		log.Printf("Failed to read packet: %v", err)
		return
	}
	if flags != 0 {
		log.Printf("Failed to read packet, flags: %v", flags)
		return
	}

	oob = oob[:oobn]
	clientRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		log.Printf("Failed to receive packet timestamp")
		clientRxTime = time.Now().UTC()
	}
	pkt.Bytes = pkt.Bytes[:n]

	err = pkt.Decode()
	if err != nil {
		log.Printf("Failed to decode packet: %v", err)
		return
	}

	udppkt, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		log.Printf("Failed to read packet payload: not a UDP packet")
		return
	}

	log.Printf("Received payload at %v via %v with flags = %v:", clientRxTime, lastHop, flags)
	fmt.Printf("%s", hex.Dump(udppkt.Payload))

	var ntpresp ntp.Packet
	err = ntp.DecodePacket(&ntpresp, udppkt.Payload)
	if err != nil {
		log.Printf("Failed to decode packet payload: %v", err)
		return
	}

	log.Printf("Received NTP packet: %+v", ntpresp)

	serverRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
	serverTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

	clockOffset := ntp.ClockOffset(clientTxTime, serverRxTime, serverTxTime, clientRxTime)
	roundTripDelay := ntp.RoundTripDelay(clientTxTime, serverRxTime, serverTxTime, clientRxTime)

	log.Printf("%s,%s clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		remoteAddr.IA, remoteAddr.Host,
		float64(clockOffset.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(clockOffset.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(roundTripDelay.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))
}

func exitWithUsage() {
	fmt.Println("<usage>")
	os.Exit(1)
}

func main() {
	var configFile string
	var daemonAddr string
	var localAddr snet.UDPAddr
	var remoteAddr snet.UDPAddr

	serverFlags := flag.NewFlagSet("server", flag.ExitOnError)
	relayFlags := flag.NewFlagSet("relay", flag.ExitOnError)
	clientFlags := flag.NewFlagSet("client", flag.ExitOnError)

	serverFlags.StringVar(&configFile, "config", "", "Config file")
	serverFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	serverFlags.Var(&localAddr, "local", "Local address")

	clientFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	clientFlags.Var(&localAddr, "local", "Local address")
	clientFlags.Var(&remoteAddr, "remote", "Remote address")

	if len(os.Args) < 2 {
		exitWithUsage()
	}

	switch os.Args[1] {
	case "server":
		err := serverFlags.Parse(os.Args[2:])
		if err != nil || serverFlags.NArg() != 0 {
			exitWithUsage()
		}
		log.Print("configFile:", configFile)
		log.Print("daemonAddr:", daemonAddr)
		log.Print("localAddr:", localAddr)
		runServer(configFile, daemonAddr, localAddr)
	case "relay":
		err := relayFlags.Parse(os.Args[2:])
		if err != nil || relayFlags.NArg() != 0 {
			exitWithUsage()
		}
	case "client":
		err := clientFlags.Parse(os.Args[2:])
		if err != nil || clientFlags.NArg() != 0 {
			exitWithUsage()
		}
		log.Print("daemonAddr:", daemonAddr)
		log.Print("localAddr:", localAddr)
		log.Print("remoteAddr:", remoteAddr)
		runClient(daemonAddr, localAddr, remoteAddr)
	default:
		exitWithUsage()
	}
}
