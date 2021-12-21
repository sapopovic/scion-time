package main

import (
	"unsafe"

	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"net"
	"time"

	"golang.org/x/sys/unix"

	"github.com/facebook/time/ntp/protocol/ntp"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/config"
	"github.com/scionproto/scion/go/lib/daemon"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	_ "example.com/scion-time/go/core/prev"

	"example.com/scion-time/go/driver"

	"example.com/scion-time/go/core"
)

type tsConfig struct {
	MBGTimeSources []string `toml:"mbg_time_sources,omitempty"`
	NTPTimeSources []string `toml:"ntp_time_sources,omitempty"`
	SCIONPeers []string `toml:"scion_peers,omitempty"`
}

type timeSource interface {
	fetchTime() (refTime time.Time, sysTime time.Time, err error)
}

type mbgTimeSource string
type ntpTimeSource string

var timeSources []timeSource

func (s mbgTimeSource) fetchTime() (time.Time, time.Time, error) {
	// TODO: return drivers.FetchMBGTime(string(s))
	return time.Time{}, time.Time{}, nil
}

func (s ntpTimeSource) fetchTime() (time.Time, time.Time, error) {
	return drivers.FetchNTPTime(string(s))
}

func newDaemonConnector(ctx context.Context, daemonAddr string) daemon.Connector {
	s := &daemon.Service{
		Address: daemonAddr,
	}
	c, err := s.Connect(ctx)
	if err != nil {
		log.Fatal("Failed to create SCION Daemon connector:", err)
	}
	return c
}

func runServer(configFile, daemonAddr string, localAddr snet.UDPAddr) {
	var err error
	ctx := context.Background()

	core.RegisterLocalClock(&core.SysClock{})

	localClock := core.LocalClockInstance()
	_ = localClock.Now()
	localClock.Adjust(0, 0, 0.0)
	localClock.Sleep(0)

	core.RegisterPLL(&core.StdPLL{})
	pll := core.PLLInstance()
	pll.Do(0, 0.0)

	var cfg tsConfig
	err = config.LoadFile(configFile, &cfg)
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

	for _, s := range timeSources {
		refTime, sysTime, err := s.fetchTime()
		if err != nil {
			log.Fatalf("Failed to fetch clock offset from %v: %v", s, err)
		}
		log.Printf("Clock offset to %v: refTime = %v, sysTime = %v", s, refTime, sysTime)
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

	pathInfos, err := core.StartPather(newDaemonConnector(ctx, daemonAddr), peerIAs)
	if err != nil {
		log.Fatal("Failed to start pather:", err)
	}
	go func() {
		for {
			<-pathInfos
		}
	}()

	err = core.StartIPServer(localAddr.IA, snet.CopyUDPAddr(localAddr.Host))
	if err != nil {
		log.Fatalf("Failed to start IP server: %v", err)
	}

	err = core.StartSCIONServer(localAddr.IA, snet.CopyUDPAddr(localAddr.Host))
	if err != nil {
		log.Fatalf("Failed to start SCION server: %v", err)
	}

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

	buf := new(bytes.Buffer)
	clientTxTime := time.Now().UTC()
	sec, frac := ntp.Time(clientTxTime)
	request := &ntp.Packet{
		Settings:   0x1B,
		TxTimeSec:  sec,
		TxTimeFrac: frac,
	}
	err = binary.Write(buf, binary.BigEndian, request)
	if err != nil {
		log.Fatalf("Failed to send NTP packet, %v", err)
	}

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
				Payload: buf.Bytes(),
			},
		},
	}

	err = pkt.Serialize()
	if err != nil {
		log.Printf("Failed to serialize packet: %v", err)
		return
	}

	nextHop := sp.UnderlayNextHop()
	if nextHop == nil && remoteAddr.IA.Equal(localAddr.IA) {
		nextHop = &net.UDPAddr{
			IP: remoteAddr.Host.IP,
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

	err = ntp.EnableKernelTimestampsSocket(conn)
	if err != nil {
		log.Fatalf("Failed to enable kernel timestamping for packets: %v", err)
	}

	_, err = conn.Write(pkt.Bytes)
	if err != nil {
		log.Printf("Failed to write packet: %v", err)
		return
	}

	pkt.Prepare()
	oob := make([]byte, ntp.ControlHeaderSizeBytes)

	n, oobn, flags, lastHop, err := conn.ReadMsgUDP(pkt.Bytes, oob)
	if err != nil {
		log.Printf("Failed to read packet: %v", err)
		return
	}

	var clientRxTime time.Time
	if oobn != 0 {
		ts := (*unix.Timespec)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
		clientRxTime = time.Unix(ts.Unix())
	} else {
		log.Printf("Failed to receive kernel timestamp")
		clientRxTime = time.Now().UTC()
	}

	pkt.Bytes = pkt.Bytes[:n]
	err = pkt.Decode()
	if err != nil {
		log.Printf("Failed to decode packet: %v", err)
		return
	}

	pld, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		log.Printf("Failed to read packet payload")
		return
	}

	log.Printf("Received payload at %v via %v with flags = %v: \"%v\":", clientRxTime, lastHop, flags)
	fmt.Printf("%s", hex.Dump(pld.Payload))

	ntpresp, err := ntp.BytesToPacket(pld.Payload)
	if err != nil {
		log.Printf("Failed to decode packet payload: %v", err)
		return
	}

	log.Printf("Received NTP packet: %+v", ntpresp)

	serverRxTime := ntp.Unix(ntpresp.RxTimeSec, ntpresp.RxTimeFrac)
	serverTxTime := ntp.Unix(ntpresp.TxTimeSec, ntpresp.TxTimeFrac)

	avgNetworkDelay := ntp.AvgNetworkDelay(clientTxTime, serverRxTime, serverTxTime, clientRxTime)
	currentRealTime := ntp.CurrentRealTime(serverTxTime, avgNetworkDelay)
	offset := ntp.CalculateOffset(currentRealTime, time.Now().UTC())

	log.Printf("Stratum: %d, Current time: %s", ntpresp.Stratum, currentRealTime)
	log.Printf("Offset: %fs (%fms), Network delay: %fs (%fms)",
		float64(offset)/float64(time.Second.Nanoseconds()),
		float64(offset)/float64(time.Millisecond.Nanoseconds()),
		float64(avgNetworkDelay)/float64(time.Second.Nanoseconds()),
		float64(avgNetworkDelay)/float64(time.Millisecond.Nanoseconds()))
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
