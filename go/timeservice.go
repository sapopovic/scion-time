// SCION time service

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/scionproto/scion/go/lib/config"
	"github.com/scionproto/scion/go/lib/daemon"
	"github.com/scionproto/scion/go/lib/snet"

	"example.com/scion-time/go/core"
	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/core/timemath"

	mbgd "example.com/scion-time/go/driver/mbg"
	ntpd "example.com/scion-time/go/driver/ntp"

	"example.com/scion-time/go/benchmark"
	"example.com/scion-time/go/tool"
)

const (
	refClockImpact       = 1.25
	refClockCutoff       = 0
	refClockSyncTimeout  = 5 * time.Second
	refClockSyncInterval = 10 * time.Second
	netClockImpact       = 2.5
	netClockCutoff       = time.Microsecond
	netClockSyncTimeout  = 5 * time.Second
	netClockSyncInterval = 60 * time.Second
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

	peers    []core.UDPAddr
	pathInfo core.PathInfo

	refcc core.ReferenceClockClient
	netcc core.NetworkClockClient
)

func runMonitor() {
	p := pprof.Lookup("threadcreate")
	for {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		log.Printf("[monitor] TotalAlloc: %v, Mallocs: %v, Frees: %v, NumGC: %v, Thread Count: %v",
			m.TotalAlloc, m.Mallocs, m.Frees, m.NumGC, p.Count())
		time.Sleep(15 * time.Second)
	}
}

func (s mbgTimeSource) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	return mbgd.MeasureClockOffset(ctx, string(s))
}

func (s ntpTimeSource) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	offset, _, err := ntpd.MeasureClockOffset(ctx, string(s))
	return offset, err
}

func loadConfig(configFile string) {
	if configFile != "" {
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
		for _, s := range cfg.SCIONPeers {
			log.Print("scion_peer: ", s)
			addr, err := snet.ParseUDPAddr(s)
			if err != nil {
				log.Fatalf("Failed to parse peer address \"%s\": %v", s, err)
			}
			peers = append(peers, core.UDPAddr{addr.IA, addr.Host})
		}
	}
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

func syncToRefClock(lclk timebase.LocalClock) {
	const (
		initDuration = 64.0
		initPackets  = 6.0
		pollPeriod   = 64.0
	)
	pll := core.NewStandardPLL(lclk)
	ref := fmt.Sprintf("%v", timeSources[0])
	numRefs := 1.0
	t0 := 1.0
	for {
		offset, weight, err := ntpd.MeasureClockOffset(context.Background(), ref)
		if err != nil {
			log.Printf("Failed to measure clock offset to %v: %v", ref, err)
		} else {
			pll.Do(offset, weight)
		}
		d := pollPeriod / numRefs
		if t0 < initDuration {
			dt := math.Exp(math.Log(initDuration) / (initPackets * numRefs))
			if t0*dt < initDuration {
				d = t0*dt - t0
			}
		}
		t0 += d
		lclk.Sleep(timemath.Duration(d))
	}
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
	for {
		corr, err := measureOffsetToRefClock(timeSources, refClockSyncTimeout)
		if err == nil && timemath.Abs(corr) > refClockCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			lclk.Adjust(corr, refClockSyncInterval, 0)
		}
		lclk.Sleep(refClockSyncInterval)
	}
}

func measureOffsetToNetClock(peers []core.UDPAddr, pi core.PathInfo,
	timeout time.Duration) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return netcc.MeasureClockOffset(ctx, peers, pi)
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
	for {
		corr, err := measureOffsetToNetClock(peers, pathInfo, netClockSyncTimeout)
		if err == nil && timemath.Abs(corr) > netClockCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			lclk.Adjust(corr, netClockSyncInterval, 0)
		}
		lclk.Sleep(netClockSyncInterval)
	}
}

func runServer(configFile, daemonAddr string, localAddr snet.UDPAddr) {
	loadConfig(configFile)

	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)

	if len(timeSources) != 0 {
		syncToRefClock(lclk)
		go runLocalClockSync(lclk)
	}

	if len(peers) != 0 {
		netcc.SetLocalHost(snet.CopyUDPAddr(localAddr.Host))

		pathInfos, err := core.StartPather(newDaemonConnector(daemonAddr), peers)
		if err != nil {
			log.Fatal("Failed to start pather:", err)
		}
		go handlePathInfos(pathInfos)

		go runGlobalClockSync(lclk)
	}

	err := core.StartIPServer(snet.CopyUDPAddr(localAddr.Host))
	if err != nil {
		log.Fatalf("Failed to start IP server: %v", err)
	}
	err = core.StartSCIONServer(localAddr.IA, snet.CopyUDPAddr(localAddr.Host))
	if err != nil {
		log.Fatalf("Failed to start SCION server: %v", err)
	}

	select {}
}

func runRelay(configFile string, localAddr snet.UDPAddr) {
	loadConfig(configFile)

	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)

	if len(timeSources) != 0 {
		syncToRefClock(lclk)
		go runLocalClockSync(lclk)
	}

	if len(peers) != 0 {
		log.Fatalf("Unexpected configuration: scion_peers=%v", peers)
	}

	err := core.StartIPServer(snet.CopyUDPAddr(localAddr.Host))
	if err != nil {
		log.Fatalf("Failed to start IP server: %v", err)
	}
	err = core.StartSCIONServer(localAddr.IA, snet.CopyUDPAddr(localAddr.Host))
	if err != nil {
		log.Fatalf("Failed to start SCION server: %v", err)
	}

	select {}
}

func runClient(configFile string) {
	loadConfig(configFile)

	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)

	if len(timeSources) != 0 {
		syncToRefClock(lclk)
		go runLocalClockSync(lclk)
	}

	if len(peers) != 0 {
		log.Fatalf("Unexpected configuration: scion_peers=%v", peers)
	}

	select {}
}

func runIPTool(localAddr, remoteAddr snet.UDPAddr) {
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
	tool.RunIPClient(localAddr.Host, remoteAddr.Host)
}

func runSCIONTool(daemonAddr string, localAddr, remoteAddr snet.UDPAddr) {
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
	tool.RunSCIONClient(daemonAddr, localAddr, remoteAddr)
}

func runIPBenchmark(localAddr, remoteAddr snet.UDPAddr) {
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
	benchmark.RunIPBenchmark(localAddr.Host, remoteAddr.Host)
}

func runSCIONBenchmark(daemonAddr string, localAddr, remoteAddr snet.UDPAddr) {
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
	benchmark.RunSCIONBenchmark(daemonAddr, localAddr, remoteAddr)
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

	serverFlags := flag.NewFlagSet("server", flag.ExitOnError)
	relayFlags := flag.NewFlagSet("relay", flag.ExitOnError)
	clientFlags := flag.NewFlagSet("client", flag.ExitOnError)
	toolFlags := flag.NewFlagSet("tool", flag.ExitOnError)
	benchmarkFlags := flag.NewFlagSet("benchmark", flag.ExitOnError)

	serverFlags.StringVar(&configFile, "config", "", "Config file")
	serverFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	serverFlags.Var(&localAddr, "local", "Local address")

	relayFlags.StringVar(&configFile, "config", "", "Config file")
	relayFlags.Var(&localAddr, "local", "Local address")

	clientFlags.StringVar(&configFile, "config", "", "Config file")
	clientFlags.Var(&localAddr, "local", "Local address")

	toolFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	toolFlags.Var(&localAddr, "local", "Local address")
	toolFlags.Var(&remoteAddr, "remote", "Remote address")

	benchmarkFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	benchmarkFlags.Var(&localAddr, "local", "Local address")
	benchmarkFlags.Var(&remoteAddr, "remote", "Remote address")

	if len(os.Args) < 2 {
		exitWithUsage()
	}

	switch os.Args[1] {
	case serverFlags.Name():
		err := serverFlags.Parse(os.Args[2:])
		if err != nil || serverFlags.NArg() != 0 {
			exitWithUsage()
		}
		log.Print("configFile:", configFile)
		log.Print("daemonAddr:", daemonAddr)
		log.Print("localAddr:", localAddr)
		runServer(configFile, daemonAddr, localAddr)
	case relayFlags.Name():
		err := relayFlags.Parse(os.Args[2:])
		if err != nil || relayFlags.NArg() != 0 {
			exitWithUsage()
		}
		log.Print("configFile:", configFile)
		log.Print("localAddr:", localAddr)
		runRelay(configFile, localAddr)
	case clientFlags.Name():
		err := clientFlags.Parse(os.Args[2:])
		if err != nil || clientFlags.NArg() != 0 {
			exitWithUsage()
		}
		log.Print("configFile:", configFile)
		runClient(configFile)
	case toolFlags.Name():
		err := toolFlags.Parse(os.Args[2:])
		if err != nil || toolFlags.NArg() != 0 {
			exitWithUsage()
		}
		log.Print("daemonAddr:", daemonAddr)
		log.Print("localAddr:", localAddr)
		log.Print("remoteAddr:", remoteAddr)
		if !localAddr.IA.IsZero() && !remoteAddr.IA.IsZero() {
			runSCIONTool(daemonAddr, localAddr, remoteAddr)
		} else if localAddr.IA.IsZero() && remoteAddr.IA.IsZero() {
			if daemonAddr != "" {
				exitWithUsage()
			}
			runIPTool(localAddr, remoteAddr)
		} else {
			exitWithUsage()
		}
	case benchmarkFlags.Name():
		err := benchmarkFlags.Parse(os.Args[2:])
		if err != nil || benchmarkFlags.NArg() != 0 {
			exitWithUsage()
		}
		log.Print("daemonAddr:", daemonAddr)
		log.Print("localAddr:", localAddr)
		log.Print("remoteAddr:", remoteAddr)
		if !localAddr.IA.IsZero() && !remoteAddr.IA.IsZero() {
			runSCIONBenchmark(daemonAddr, localAddr, remoteAddr)
		} else if localAddr.IA.IsZero() && remoteAddr.IA.IsZero() {
			if daemonAddr != "" {
				exitWithUsage()
			}
			runIPBenchmark(localAddr, remoteAddr)
		} else {
			exitWithUsage()
		}
	case "x":
		runX()
	default:
		exitWithUsage()
	}
}
