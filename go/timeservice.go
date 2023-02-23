// SCION time service

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/pelletier/go-toml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/snet"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

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
	log *zap.Logger
	dev string
}

type ntpReferenceClockIP struct {
	ntpc       *ntpd.IPClient
	localAddr  *net.UDPAddr
	remoteAddr *net.UDPAddr
}

type ntpReferenceClockSCION struct {
	ntpcs      [5]*ntpd.SCIONClient
	localAddr  udp.UDPAddr
	remoteAddr udp.UDPAddr
	pather     *core.Pather
}

type localReferenceClock struct{}

var (
	log *zap.Logger

	refClocks       []core.ReferenceClock
	refClockOffsets []time.Duration
	refClockClient  core.ReferenceClockClient
	netClocks       []core.ReferenceClock
	netClockOffsets []time.Duration
	netClockClient  core.ReferenceClockClient
)

func initLogger(verbose bool) {
	c := zap.NewDevelopmentConfig()
	c.DisableStacktrace = true
	c.EncoderConfig.EncodeCaller = func(
		caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
		// See https://github.com/scionproto/scion/blob/master/pkg/log/log.go
		p := caller.TrimmedPath()
		if len(p) > 30 {
			p = "..." + p[len(p)-27:]
		}
		enc.AppendString(fmt.Sprintf("%30s", p))
	}
	if !verbose {
		c.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	var err error
	log, err = c.Build()
	if err != nil {
		panic(err)
	}
	refClockClient.Log = log
	netClockClient.Log = log
}

func runMonitor(log *zap.Logger) {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe("127.0.0.1:8080", nil)
	log.Fatal("failed to serve metrics", zap.Error(err))
}

func (c *mbgReferenceClock) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	return mbgd.MeasureClockOffset(ctx, c.log, c.dev)
}

func (c *mbgReferenceClock) String() string {
	return fmt.Sprintf("mbg reference clock at %s", c.dev)
}

func newNTPRefernceClockIP(localAddr, remoteAddr *net.UDPAddr) *ntpReferenceClockIP {
	c := &ntpReferenceClockIP{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
	c.ntpc = &ntpd.IPClient{
		Log:             log,
		InterleavedMode: true,
	}
	return c
}

func (c *ntpReferenceClockIP) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	return core.MeasureClockOffsetIP(ctx, c.ntpc, c.localAddr, c.remoteAddr)
}

func (c *ntpReferenceClockIP) String() string {
	return fmt.Sprintf("NTP reference clock (IP) at %s", c.remoteAddr)
}

func newNTPRefernceClockSCION(localAddr, remoteAddr udp.UDPAddr) *ntpReferenceClockSCION {
	c := &ntpReferenceClockSCION{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
	for i := 0; i != len(c.ntpcs); i++ {
		c.ntpcs[i] = &ntpd.SCIONClient{
			Log:             log,
			InterleavedMode: true,
		}
	}
	return c
}

func (c *ntpReferenceClockSCION) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	paths := c.pather.Paths(c.remoteAddr.IA)
	return core.MeasureClockOffsetSCION(ctx, c.ntpcs[:], c.localAddr, c.remoteAddr, paths)
}

func (c *ntpReferenceClockSCION) String() string {
	return fmt.Sprintf("NTP reference clock (SCION) at %s", c.remoteAddr)
}

func (c *localReferenceClock) MeasureClockOffset(ctx context.Context) (time.Duration, error) {
	return 0, nil
}

func (c *localReferenceClock) String() string {
	return "local reference clock"
}

func newDaemonConnector(ctx context.Context, log *zap.Logger, daemonAddr string) daemon.Connector {
	s := &daemon.Service{
		Address: daemonAddr,
	}
	c, err := s.Connect(ctx)
	if err != nil {
		log.Fatal("failed to create demon connector", zap.Error(err))
	}
	return c
}

func loadConfig(ctx context.Context, log *zap.Logger,
	configFile, daemonAddr string, localAddr *snet.UDPAddr) {
	if configFile != "" {
		var cfg svcConfig
		raw, err := os.ReadFile(configFile)
		if err != nil {
			log.Fatal("failed to load configuration", zap.Error(err))
		}
		err = toml.NewDecoder(bytes.NewReader(raw)).Strict(true).Decode(&cfg)
		if err != nil {
			log.Fatal("failed to decode configuration", zap.Error(err))
		}
		for _, s := range cfg.MBGReferenceClocks {
			refClocks = append(refClocks, &mbgReferenceClock{
				log: log,
				dev: s,
			})
		}
		var dstIAs []addr.IA
		for _, s := range cfg.NTPReferenceClocks {
			remoteAddr, err := snet.ParseUDPAddr(s)
			if err != nil {
				log.Fatal("failed to parse reference clock address",
					zap.String("address", s), zap.Error(err))
			}
			if !remoteAddr.IA.IsZero() {
				refClocks = append(refClocks, newNTPRefernceClockSCION(
					udp.UDPAddrFromSnet(localAddr),
					udp.UDPAddrFromSnet(remoteAddr),
				))
				dstIAs = append(dstIAs, remoteAddr.IA)
			} else {
				refClocks = append(refClocks, newNTPRefernceClockIP(
					localAddr.Host,
					remoteAddr.Host,
				))
			}
		}
		for _, s := range cfg.SCIONPeers {
			remoteAddr, err := snet.ParseUDPAddr(s)
			if err != nil {
				log.Fatal("failed to parse peer address", zap.String("address", s), zap.Error(err))
			}
			if remoteAddr.IA.IsZero() {
				log.Fatal("unexpected peer address", zap.String("address", s), zap.Error(err))
			}
			netClocks = append(netClocks, newNTPRefernceClockSCION(
				udp.UDPAddrFromSnet(localAddr),
				udp.UDPAddrFromSnet(remoteAddr),
			))
			dstIAs = append(dstIAs, remoteAddr.IA)
		}
		if len(netClocks) != 0 {
			netClocks = append(netClocks, &localReferenceClock{})
		}
		if daemonAddr != "" {
			dc := newDaemonConnector(ctx, log, daemonAddr)
			pather := core.StartPather(log, dc, dstIAs)
			drkeyFetcher := drkeyutil.NewFetcher(dc)
			for _, c := range refClocks {
				scionclk, ok := c.(*ntpReferenceClockSCION)
				if ok {
					scionclk.pather = pather
					for i := 0; i != len(scionclk.ntpcs); i++ {
						scionclk.ntpcs[i].DRKeyFetcher = drkeyFetcher
					}
				}
			}
			for _, c := range netClocks {
				scionclk, ok := c.(*ntpReferenceClockSCION)
				if ok {
					scionclk.pather = pather
					for i := 0; i != len(scionclk.ntpcs); i++ {
						scionclk.ntpcs[i].DRKeyFetcher = drkeyFetcher
					}
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

func runLocalClockSync(log *zap.Logger, lclk timebase.LocalClock) {
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
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "timeservice_global_sync_corr",
		Help: "The current clock correction applied based on local sync",
	})
	pll := core.NewPLL(log, lclk)
	for {
		corrGauge.Set(0)
		corr := measureOffsetToRefClocks(refClockSyncTimeout)
		if timemath.Abs(corr) > refClockCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			// lclk.Adjust(corr, refClockSyncInterval, 0)
			pll.Do(corr, 1000.0 /* weight */)
			corrGauge.Set(float64(corr))
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

func runGlobalClockSync(log *zap.Logger, lclk timebase.LocalClock) {
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
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "timeservice_global_sync_corr",
		Help: "The current clock correction applied based on global sync",
	})
	pll := core.NewPLL(log, lclk)
	for {
		corrGauge.Set(0)
		corr := measureOffsetToNetClocks(netClockSyncTimeout)
		if timemath.Abs(corr) > netClockCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			// lclk.Adjust(corr, netClockSyncInterval, 0)
			pll.Do(corr, 1000.0 /* weight */)
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(netClockSyncInterval)
	}
}

func runServer(configFile, daemonAddr string, localAddr *snet.UDPAddr) {
	ctx := context.Background()

	loadConfig(ctx, log, configFile, daemonAddr, localAddr)

	lclk := &core.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	if len(refClocks) != 0 {
		syncToRefClocks(lclk)
		go runLocalClockSync(log, lclk)
	}

	if len(netClocks) != 0 {
		go runGlobalClockSync(log, lclk)
	}

	core.StartIPServer(log, snet.CopyUDPAddr(localAddr.Host))
	core.StartSCIONServer(ctx, log, snet.CopyUDPAddr(localAddr.Host), daemonAddr)

	runMonitor(log)
}

func runRelay(configFile, daemonAddr string, localAddr *snet.UDPAddr) {
	ctx := context.Background()

	loadConfig(ctx, log, configFile, daemonAddr, localAddr)

	lclk := &core.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	if len(refClocks) != 0 {
		syncToRefClocks(lclk)
		go runLocalClockSync(log, lclk)
	}

	if len(netClocks) != 0 {
		log.Fatal("unexpected configuration", zap.Int("number of peers", len(netClocks)))
	}

	core.StartIPServer(log, snet.CopyUDPAddr(localAddr.Host))
	core.StartSCIONServer(ctx, log, snet.CopyUDPAddr(localAddr.Host), daemonAddr)

	runMonitor(log)
}

func runClient(configFile, daemonAddr string, localAddr *snet.UDPAddr) {
	ctx := context.Background()

	loadConfig(ctx, log, configFile, daemonAddr, localAddr)

	lclk := &core.SystemClock{Log: log}
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
		go runLocalClockSync(log, lclk)
	}

	if len(netClocks) != 0 {
		log.Fatal("unexpected configuration", zap.Int("number of peers", len(netClocks)))
	}

	runMonitor(log)
}

func runIPTool(localAddr, remoteAddr *snet.UDPAddr) {
	var err error
	ctx := context.Background()

	lclk := &core.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	c := &ntpd.IPClient{
		Log:             log,
		InterleavedMode: true,
	}
	for i := 0; i != 2; i++ {
		_, _, err = c.MeasureClockOffsetIP(ctx, localAddr.Host, remoteAddr.Host)
		if err != nil {
			log.Fatal("failed to measure clock offset", zap.Stringer("to", remoteAddr.Host), zap.Error(err))
		}
	}
}

func runSCIONTool(daemonAddr, dispatcherMode string, localAddr, remoteAddr *snet.UDPAddr) {
	var err error
	ctx := context.Background()

	lclk := &core.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	if dispatcherMode == dispatcherModeInternal {
		core.StartSCIONDisptacher(ctx, log, snet.CopyUDPAddr(localAddr.Host))
	}

	dc := newDaemonConnector(ctx, log, daemonAddr)
	ps, err := dc.Paths(ctx, remoteAddr.IA, localAddr.IA, daemon.PathReqFlags{Refresh: true})
	if err != nil {
		log.Fatal("failed to lookup paths", zap.Stringer("to", remoteAddr.IA), zap.Error(err))
	}
	if len(ps) == 0 {
		log.Fatal("no paths available", zap.Stringer("to", remoteAddr.IA))
	}
	log.Debug("available paths", zap.Stringer("to", remoteAddr.IA), zap.Array("via", scion.PathArrayMarshaler{Paths: ps}))

	sp := ps[0]
	log.Debug("selected path", zap.Stringer("to", remoteAddr.IA), zap.Object("via", scion.PathMarshaler{Path: sp}))

	laddr := udp.UDPAddrFromSnet(localAddr)
	raddr := udp.UDPAddrFromSnet(remoteAddr)
	c := &ntpd.SCIONClient{
		Log:             log,
		InterleavedMode: true,
		DRKeyFetcher:    drkeyutil.NewFetcher(dc),
	}
	for i := 0; i != 2; i++ {
		_, _, err = c.MeasureClockOffsetSCION(ctx, laddr, raddr, sp)
		if err != nil {
			log.Fatal("failed to measure clock offset",
				zap.Stringer("remoteIA", raddr.IA),
				zap.Stringer("remoteHost", raddr.Host),
				zap.Error(err),
			)
		}
	}
}

func runIPBenchmark(localAddr, remoteAddr *snet.UDPAddr) {
	lclk := &core.SystemClock{Log: zap.NewNop()}
	timebase.RegisterClock(lclk)
	benchmark.RunIPBenchmark(localAddr.Host, remoteAddr.Host)
}

func runSCIONBenchmark(daemonAddr string, localAddr, remoteAddr *snet.UDPAddr) {
	lclk := &core.SystemClock{Log: zap.NewNop()}
	timebase.RegisterClock(lclk)
	benchmark.RunSCIONBenchmark(daemonAddr, localAddr, remoteAddr)
}

func runDRKeyDemo(daemonAddr string, serverMode bool, serverAddr, clientAddr *snet.UDPAddr) {
	ctx := context.Background()
	dc := newDaemonConnector(ctx, log, daemonAddr)

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
	var (
		verbose         bool
		configFile      string
		daemonAddr      string
		localAddr       snet.UDPAddr
		remoteAddr      snet.UDPAddr
		dispatcherMode  string
		drkeyMode       string
		drkeyServerAddr snet.UDPAddr
		drkeyClientAddr snet.UDPAddr
	)

	serverFlags := flag.NewFlagSet("server", flag.ExitOnError)
	relayFlags := flag.NewFlagSet("relay", flag.ExitOnError)
	clientFlags := flag.NewFlagSet("client", flag.ExitOnError)
	toolFlags := flag.NewFlagSet("tool", flag.ExitOnError)
	benchmarkFlags := flag.NewFlagSet("benchmark", flag.ExitOnError)
	drkeyFlags := flag.NewFlagSet("drkey", flag.ExitOnError)

	serverFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	serverFlags.StringVar(&configFile, "config", "", "Config file")
	serverFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	serverFlags.Var(&localAddr, "local", "Local address")

	relayFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	relayFlags.StringVar(&configFile, "config", "", "Config file")
	relayFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	relayFlags.Var(&localAddr, "local", "Local address")

	clientFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	clientFlags.StringVar(&configFile, "config", "", "Config file")
	clientFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	clientFlags.Var(&localAddr, "local", "Local address")

	toolFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	toolFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	toolFlags.StringVar(&dispatcherMode, "dispatcher", "", "Dispatcher mode")
	toolFlags.Var(&localAddr, "local", "Local address")
	toolFlags.Var(&remoteAddr, "remote", "Remote address")

	benchmarkFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	benchmarkFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	benchmarkFlags.Var(&localAddr, "local", "Local address")
	benchmarkFlags.Var(&remoteAddr, "remote", "Remote address")

	drkeyFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
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
		initLogger(verbose)
		runServer(configFile, daemonAddr, &localAddr)
	case relayFlags.Name():
		err := relayFlags.Parse(os.Args[2:])
		if err != nil || relayFlags.NArg() != 0 {
			exitWithUsage()
		}
		initLogger(verbose)
		runRelay(configFile, daemonAddr, &localAddr)
	case clientFlags.Name():
		err := clientFlags.Parse(os.Args[2:])
		if err != nil || clientFlags.NArg() != 0 {
			exitWithUsage()
		}
		initLogger(verbose)
		runClient(configFile, daemonAddr, &localAddr)
	case toolFlags.Name():
		err := toolFlags.Parse(os.Args[2:])
		if err != nil || toolFlags.NArg() != 0 {
			exitWithUsage()
		}
		if !remoteAddr.IA.IsZero() {
			if dispatcherMode == "" {
				dispatcherMode = dispatcherModeExternal
			} else if dispatcherMode != dispatcherModeExternal &&
				dispatcherMode != dispatcherModeInternal {
				exitWithUsage()
			}
			initLogger(verbose)
			runSCIONTool(daemonAddr, dispatcherMode, &localAddr, &remoteAddr)
		} else {
			if daemonAddr != "" {
				exitWithUsage()
			}
			if dispatcherMode != "" {
				exitWithUsage()
			}
			initLogger(verbose)
			runIPTool(&localAddr, &remoteAddr)
		}
	case benchmarkFlags.Name():
		err := benchmarkFlags.Parse(os.Args[2:])
		if err != nil || benchmarkFlags.NArg() != 0 {
			exitWithUsage()
		}
		if !remoteAddr.IA.IsZero() {
			initLogger(verbose)
			runSCIONBenchmark(daemonAddr, &localAddr, &remoteAddr)
		} else {
			if daemonAddr != "" {
				exitWithUsage()
			}
			initLogger(verbose)
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
		initLogger(verbose)
		runDRKeyDemo(daemonAddr, serverMode, &drkeyServerAddr, &drkeyClientAddr)
	case "x":
		runX()
	default:
		exitWithUsage()
	}
}
