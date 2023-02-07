// SCION time service

package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	stdlog "log"
	"net"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/private/config"

	"go.uber.org/zap"

	"example.com/scion-time/go/core"
	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/core/timemath"

	"example.com/scion-time/go/drkeyutil"

	"example.com/scion-time/go/net/scion"
	"example.com/scion-time/go/net/udp"

	mbgd "example.com/scion-time/go/driver/mbg"
	ntpd "example.com/scion-time/go/driver/ntp"

	"example.com/scion-time/go/benchmark"
)

const (
	dispatcherModeExternal = "external"
	dispatcherModeInternal = "internal"

	refClockImpact       = 1.25
	refClockCutoff       = 0
	refClockSyncTimeout  = 5 * time.Second
	refClockSyncInterval = 10 * time.Second
	netClockImpact       = 2.5
	netClockCutoff       = time.Microsecond
	netClockSyncTimeout  = 5 * time.Second
	netClockSyncInterval = 60 * time.Second
)

type svcConfig struct {
	MBGReferenceClocks []string `toml:"mbg_reference_clocks,omitempty"`
	NTPReferenceClocks []string `toml:"ntp_reference_clocks,omitempty"`
	SCIONPeers         []string `toml:"scion_peers,omitempty"`
}

type mbgReferenceClock struct {
	dev string
}

type ntpReferenceClockIP struct {
	localAddr, remoteAddr *net.UDPAddr
}

type ntpReferenceClockSCION struct {
	localAddr, remoteAddr udp.UDPAddr
	pather                *core.Pather
}

type localReferenceClock struct{}

var (
	refClocks       []core.ReferenceClock
	refClockClient  core.ReferenceClockClient
	refClockOffsets []time.Duration
	netClocks       []core.ReferenceClock
	netClockClient  core.ReferenceClockClient
	netClockOffsets []time.Duration
)

func runMonitor() {
	p := pprof.Lookup("threadcreate")
	for {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		stdlog.Printf("[monitor] TotalAlloc: %v, Mallocs: %v, Frees: %v, NumGC: %v, Thread Count: %v",
			m.TotalAlloc, m.Mallocs, m.Frees, m.NumGC, p.Count())
		time.Sleep(15 * time.Second)
	}
}

func (c *mbgReferenceClock) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	return mbgd.MeasureClockOffset(ctx, c.dev)
}

func (c *ntpReferenceClockIP) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	offset, _, err := ntpd.MeasureClockOffsetIP(ctx, c.localAddr, c.remoteAddr)
	return offset, err
}

func (c *ntpReferenceClockSCION) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	paths := c.pather.Paths(c.remoteAddr.IA)
	offset, err := core.MeasureClockOffsetSCION(ctx, c.localAddr, c.remoteAddr, paths)
	return offset, err
}

func (c *localReferenceClock) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	return 0, nil
}

func newDaemonConnector(ctx context.Context, daemonAddr string) daemon.Connector {
	s := &daemon.Service{
		Address: daemonAddr,
	}
	c, err := s.Connect(ctx)
	if err != nil {
		stdlog.Fatalf("Failed to create SCION Daemon connector: %v", err)
	}
	return c
}

func loadConfig(configFile, daemonAddr string, localAddr *snet.UDPAddr) {
	if configFile != "" {
		var cfg svcConfig
		err := config.LoadFile(configFile, &cfg)
		if err != nil {
			stdlog.Fatalf("Failed to load configuration: %v", err)
		}
		for _, s := range cfg.MBGReferenceClocks {
			stdlog.Print("mbg_refernce_clock: ", s)
			refClocks = append(refClocks, &mbgReferenceClock{
				dev: s,
			})
		}
		var dstIAs []addr.IA
		for _, s := range cfg.NTPReferenceClocks {
			stdlog.Print("ntp_reference_clock: ", s)
			remoteAddr, err := snet.ParseUDPAddr(s)
			if err != nil {
				stdlog.Fatalf("Failed to parse reference clock address: %v", err)
			}
			if !remoteAddr.IA.IsZero() {
				refClocks = append(refClocks, &ntpReferenceClockSCION{
					localAddr:  udp.UDPAddrFromSnet(localAddr),
					remoteAddr: udp.UDPAddrFromSnet(remoteAddr),
				})
				dstIAs = append(dstIAs, remoteAddr.IA)
			} else {
				refClocks = append(refClocks, &ntpReferenceClockIP{
					localAddr:  localAddr.Host,
					remoteAddr: remoteAddr.Host,
				})
			}
		}
		for _, s := range cfg.SCIONPeers {
			stdlog.Print("scion_peer: ", s)
			remoteAddr, err := snet.ParseUDPAddr(s)
			if err != nil {
				stdlog.Fatalf("Failed to parse peer address %v", err)
			}
			if remoteAddr.IA.IsZero() {
				stdlog.Fatalf("Unexpected SCION address \"%s\"", s)
			}
			netClocks = append(netClocks, &ntpReferenceClockSCION{
				localAddr:  udp.UDPAddrFromSnet(localAddr),
				remoteAddr: udp.UDPAddrFromSnet(remoteAddr),
			})
			dstIAs = append(dstIAs, remoteAddr.IA)
		}
		if len(netClocks) != 0 {
			netClocks = append(netClocks, &localReferenceClock{})
		}
		if daemonAddr != "" {
			pather := core.StartPather(newDaemonConnector(context.Background(), daemonAddr), dstIAs)
			for _, c := range refClocks {
				scionclk, ok := c.(*ntpReferenceClockSCION)
				if ok {
					scionclk.pather = pather
				}
			}
			for _, c := range netClocks {
				scionclk, ok := c.(*ntpReferenceClockSCION)
				if ok {
					scionclk.pather = pather
				}
			}
		}
		refClockOffsets = make([]time.Duration, len(refClocks))
		netClockOffsets = make([]time.Duration, len(netClocks))
	}
}

func measureOffsetToRefClocks(timeout time.Duration) time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	refClockClient.MeasureClockOffsets(ctx, refClocks, refClockOffsets)
	return timemath.Median(refClockOffsets)
}

func syncToRefClocks(lclk timebase.LocalClock) {
	corr := measureOffsetToRefClocks(refClockSyncTimeout)
	if corr != 0 {
		lclk.Step(corr)
	}
}

func runLocalClockSync(lclk timebase.LocalClock) {
	if refClockImpact <= 1.0 {
		panic("invalid reference clock impact factor")
	}
	if refClockSyncInterval <= 0 {
		panic("invalid reference clock sync interval")
	}
	if refClockSyncTimeout < 0 || refClockSyncTimeout > refClockSyncInterval/2 {
		panic("invalid reference clock sync timeout")
	}
	maxCorr := refClockImpact * float64(lclk.MaxDrift(refClockSyncInterval))
	if maxCorr <= 0 {
		panic("invalid reference clock max correction")
	}
	pll := core.NewPLL(lclk)
	for {
		corr := measureOffsetToRefClocks(refClockSyncTimeout)
		if timemath.Abs(corr) > refClockCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			// lclk.Adjust(corr, refClockSyncInterval, 0)
			pll.Do(corr, 1000.0 /* weight */)
		}
		lclk.Sleep(refClockSyncInterval)
	}
}

func measureOffsetToNetClocks(timeout time.Duration) time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	netClockClient.MeasureClockOffsets(ctx, netClocks, netClockOffsets)
	return timemath.FaultTolerantMidpoint(netClockOffsets)
}

func runGlobalClockSync(lclk timebase.LocalClock) {
	if netClockImpact <= 1.0 {
		panic("invalid network clock impact factor")
	}
	if netClockImpact-1.0 <= refClockImpact {
		panic("invalid network clock impact factor")
	}
	if netClockSyncInterval < refClockSyncInterval {
		panic("invalid network clock sync interval")
	}
	if netClockSyncTimeout < 0 || netClockSyncTimeout > netClockSyncInterval/2 {
		panic("invalid network clock sync timeout")
	}
	maxCorr := netClockImpact * float64(lclk.MaxDrift(netClockSyncInterval))
	if maxCorr <= 0 {
		panic("invalid network clock max correction")
	}
	pll := core.NewPLL(lclk)
	for {
		corr := measureOffsetToNetClocks(netClockSyncTimeout)
		if timemath.Abs(corr) > netClockCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			// lclk.Adjust(corr, netClockSyncInterval, 0)
			pll.Do(corr, 1000.0 /* weight */)
		}
		lclk.Sleep(netClockSyncInterval)
	}
}

func runServer(configFile, daemonAddr string, localAddr *snet.UDPAddr) {
	ctx := context.Background()
	log := zap.Must(zap.NewDevelopment())

	loadConfig(configFile, daemonAddr, localAddr)

	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)

	if len(refClocks) != 0 {
		syncToRefClocks(lclk)
		go runLocalClockSync(lclk)
	}

	if len(netClocks) != 0 {
		go runGlobalClockSync(lclk)
	}

	core.StartIPServer(snet.CopyUDPAddr(localAddr.Host))
	core.StartSCIONServer(ctx, log, snet.CopyUDPAddr(localAddr.Host), daemonAddr)

	select {}
}

func runRelay(configFile, daemonAddr string, localAddr *snet.UDPAddr) {
	ctx := context.Background()
	log := zap.Must(zap.NewDevelopment())

	loadConfig(configFile, daemonAddr, localAddr)

	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)

	if len(refClocks) != 0 {
		syncToRefClocks(lclk)
		go runLocalClockSync(lclk)
	}

	if len(netClocks) != 0 {
		stdlog.Fatalf("Unexpected configuration: scion_peers=%v", netClocks)
	}

	core.StartIPServer(snet.CopyUDPAddr(localAddr.Host))
	core.StartSCIONServer(ctx, log, snet.CopyUDPAddr(localAddr.Host), daemonAddr)

	select {}
}

func runClient(configFile, daemonAddr string, localAddr *snet.UDPAddr) {
	ctx := context.Background()
	log := zap.Must(zap.NewDevelopment())

	loadConfig(configFile, daemonAddr, localAddr)

	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)

	scionClocksAvailable := false
	for _, c := range refClocks {
		_, ok := c.(*ntpReferenceClockSCION)
		if ok {
			scionClocksAvailable = true
			break
		}
	}
	if scionClocksAvailable {
		core.StartSCIONDisptacher(ctx, log, snet.CopyUDPAddr(localAddr.Host))
	}

	if len(refClocks) != 0 {
		syncToRefClocks(lclk)
		go runLocalClockSync(lclk)
	}

	if len(netClocks) != 0 {
		stdlog.Fatalf("Unexpected configuration: scion_peers=%v", netClocks)
	}

	select {}
}

func runIPTool(localAddr, remoteAddr *snet.UDPAddr) {
	var err error
	ctx := context.Background()
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
	c := &ntpd.IPClient{InterleavedMode: true}
	for n := 2; n != 0; n-- {
		_, _, err = c.MeasureClockOffsetIP(ctx, localAddr.Host, remoteAddr.Host)
		if err != nil {
			stdlog.Fatalf("Failed to measure clock offset to %s: %v", remoteAddr.Host, err)
		}
		lclk.Sleep(125 * time.Microsecond)
	}
}

func runSCIONTool(daemonAddr, dispatcherMode string, localAddr, remoteAddr *snet.UDPAddr) {
	var err error
	ctx := context.Background()
	log := zap.Must(zap.NewDevelopment())

	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)

	if dispatcherMode == dispatcherModeInternal {
		core.StartSCIONDisptacher(ctx, log, snet.CopyUDPAddr(localAddr.Host))
	}

	dc := newDaemonConnector(ctx, daemonAddr)
	ps, err := dc.Paths(ctx, remoteAddr.IA, localAddr.IA, daemon.PathReqFlags{Refresh: true})
	if err != nil {
		stdlog.Fatalf("Failed to lookup paths: %v:", err)
	}
	if len(ps) == 0 {
		stdlog.Fatalf("No paths to %v available", remoteAddr.IA)
	}
	stdlog.Printf("Available paths to %v:", remoteAddr.IA)
	for _, p := range ps {
		stdlog.Printf("\t%v", p)
	}
	sp := ps[0]
	stdlog.Printf("Selected path to %v:", remoteAddr.IA)
	stdlog.Printf("\t%v", sp)
	laddr := udp.UDPAddrFromSnet(localAddr)
	raddr := udp.UDPAddrFromSnet(remoteAddr)
	c := &ntpd.SCIONClient{
		InterleavedMode: true,
		DRKeyFetcher:    drkeyutil.NewFetcher(dc),
	}
	for n := 2; n != 0; n-- {
		_, _, err = c.MeasureClockOffsetSCION(ctx, laddr, raddr, sp)
		if err != nil {
			stdlog.Fatalf("Failed to measure clock offset to %s,%s: %v", raddr.IA, raddr.Host, err)
		}
		lclk.Sleep(125 * time.Microsecond)
	}
}

func runIPBenchmark(localAddr, remoteAddr *snet.UDPAddr) {
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
	benchmark.RunIPBenchmark(localAddr.Host, remoteAddr.Host)
}

func runSCIONBenchmark(daemonAddr string, localAddr, remoteAddr *snet.UDPAddr) {
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
	benchmark.RunSCIONBenchmark(daemonAddr, localAddr, remoteAddr)
}

func runDRKeyDemo(daemonAddr string, serverMode bool, serverAddr, clientAddr *snet.UDPAddr) {
	ctx := context.Background()
	dc := newDaemonConnector(ctx, daemonAddr)

	meta := drkey.HostHostMeta{
		ProtoId:  scion.DRKeyProtoIdTS,
		Validity: time.Now(),
		SrcIA:    serverAddr.IA,
		DstIA:    clientAddr.IA,
		SrcHost:  serverAddr.Host.IP.String(),
		DstHost:  clientAddr.Host.IP.String(),
	}

	if serverMode {
		sv, err := drkeyutil.FetchSecretValue(ctx, dc, drkey.SecretValueMeta{
			Validity: meta.Validity,
			ProtoId:  meta.ProtoId,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error fetching secret value:", err)
			return
		}
		t0 := time.Now()
		serverKey, err := drkeyutil.DeriveHostHostKey(sv, meta)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error deriving key:", err)
			return
		}
		durationServer := time.Since(t0)

		fmt.Printf(
			"Server,\thost key = %s\tduration = %s\n",
			hex.EncodeToString(serverKey.Key[:]),
			durationServer,
		)
	} else {
		t0 := time.Now()
		clientKey, err := dc.DRKeyGetHostHostKey(ctx, meta)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error fetching key:", err)
			return
		}
		durationClient := time.Since(t0)

		fmt.Printf(
			"Client,\thost key = %s\tduration = %s\n",
			hex.EncodeToString(clientKey.Key[:]),
			durationClient,
		)
	}
}

func exitWithUsage() {
	fmt.Println("<usage>")
	os.Exit(1)
}

func main() {
	go runMonitor()

	var configFile string
	var daemonAddr string
	var localAddr snet.UDPAddr
	var remoteAddr snet.UDPAddr
	var dispatcherMode string
	var drkeyMode string
	var drkeyServerAddr snet.UDPAddr
	var drkeyClientAddr snet.UDPAddr

	serverFlags := flag.NewFlagSet("server", flag.ExitOnError)
	relayFlags := flag.NewFlagSet("relay", flag.ExitOnError)
	clientFlags := flag.NewFlagSet("client", flag.ExitOnError)
	toolFlags := flag.NewFlagSet("tool", flag.ExitOnError)
	benchmarkFlags := flag.NewFlagSet("benchmark", flag.ExitOnError)
	drkeyFlags := flag.NewFlagSet("drkey", flag.ExitOnError)

	serverFlags.StringVar(&configFile, "config", "", "Config file")
	serverFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	serverFlags.Var(&localAddr, "local", "Local address")

	relayFlags.StringVar(&configFile, "config", "", "Config file")
	relayFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	relayFlags.Var(&localAddr, "local", "Local address")

	clientFlags.StringVar(&configFile, "config", "", "Config file")
	clientFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	clientFlags.Var(&localAddr, "local", "Local address")

	toolFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	toolFlags.StringVar(&dispatcherMode, "dispatcher", "", "Dispatcher mode")
	toolFlags.Var(&localAddr, "local", "Local address")
	toolFlags.Var(&remoteAddr, "remote", "Remote address")

	benchmarkFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	benchmarkFlags.Var(&localAddr, "local", "Local address")
	benchmarkFlags.Var(&remoteAddr, "remote", "Remote address")

	drkeyFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	drkeyFlags.StringVar(&drkeyMode, "mode", "", "Mode")
	drkeyFlags.Var(&drkeyServerAddr, "server", "Server address")
	drkeyFlags.Var(&drkeyClientAddr, "client", "Client address")

	if len(os.Args) < 2 {
		exitWithUsage()
	}

	switch os.Args[1] {
	case serverFlags.Name():
		err := serverFlags.Parse(os.Args[2:])
		if err != nil || serverFlags.NArg() != 0 {
			exitWithUsage()
		}
		stdlog.Print("configFile:", configFile)
		stdlog.Print("daemonAddr:", daemonAddr)
		stdlog.Print("localAddr:", localAddr)
		runServer(configFile, daemonAddr, &localAddr)
	case relayFlags.Name():
		err := relayFlags.Parse(os.Args[2:])
		if err != nil || relayFlags.NArg() != 0 {
			exitWithUsage()
		}
		stdlog.Print("configFile:", configFile)
		stdlog.Print("daemonAddr:", daemonAddr)
		stdlog.Print("localAddr:", localAddr)
		runRelay(configFile, daemonAddr, &localAddr)
	case clientFlags.Name():
		err := clientFlags.Parse(os.Args[2:])
		if err != nil || clientFlags.NArg() != 0 {
			exitWithUsage()
		}
		stdlog.Print("configFile:", configFile)
		stdlog.Print("daemonAddr:", daemonAddr)
		stdlog.Print("localAddr:", localAddr)
		runClient(configFile, daemonAddr, &localAddr)
	case toolFlags.Name():
		err := toolFlags.Parse(os.Args[2:])
		if err != nil || toolFlags.NArg() != 0 {
			exitWithUsage()
		}
		stdlog.Print("daemonAddr:", daemonAddr)
		stdlog.Print("dispatcherMode:", dispatcherMode)
		stdlog.Print("localAddr:", localAddr)
		stdlog.Print("remoteAddr:", remoteAddr)
		if !remoteAddr.IA.IsZero() {
			if dispatcherMode == "" {
				dispatcherMode = dispatcherModeExternal
			} else if dispatcherMode != dispatcherModeExternal &&
				dispatcherMode != dispatcherModeInternal {
				exitWithUsage()
			}
			runSCIONTool(daemonAddr, dispatcherMode, &localAddr, &remoteAddr)
		} else {
			if daemonAddr != "" {
				exitWithUsage()
			}
			if dispatcherMode != "" {
				exitWithUsage()
			}
			runIPTool(&localAddr, &remoteAddr)
		}
	case benchmarkFlags.Name():
		err := benchmarkFlags.Parse(os.Args[2:])
		if err != nil || benchmarkFlags.NArg() != 0 {
			exitWithUsage()
		}
		stdlog.Print("daemonAddr:", daemonAddr)
		stdlog.Print("localAddr:", localAddr)
		stdlog.Print("remoteAddr:", remoteAddr)
		if !remoteAddr.IA.IsZero() {
			runSCIONBenchmark(daemonAddr, &localAddr, &remoteAddr)
		} else {
			if daemonAddr != "" {
				exitWithUsage()
			}
			runIPBenchmark(&localAddr, &remoteAddr)
		}
	case drkeyFlags.Name():
		err := drkeyFlags.Parse(os.Args[2:])
		if err != nil || drkeyFlags.NArg() != 0 {
			exitWithUsage()
		}
		if drkeyMode != "server" && drkeyMode != "client" {
			exitWithUsage()
		}
		serverMode := drkeyMode == "server"
		runDRKeyDemo(daemonAddr, serverMode, &drkeyServerAddr, &drkeyClientAddr)
	case "x":
		runX()
	default:
		exitWithUsage()
	}
}
