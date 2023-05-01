package benchmark

import (
	"context"
	"crypto/tls"
	"errors"
	"math"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"example.com/scion-time/base/metrics"
	"example.com/scion-time/base/timemath"
	"example.com/scion-time/core/config"
	"example.com/scion-time/core/timebase"

	"example.com/scion-time/net/ntp"
	"example.com/scion-time/net/nts"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/udp"
)

var (
	errWrite                  = errors.New("failed to write packet")
	errUnexpectedPacketFlags  = errors.New("failed to read packet: unexpected flags")
	errUnexpectedPacketSource = errors.New("failed to read packet: unexpected source")
	errUnexpectedPacket       = errors.New("failed to read packet: unexpected type or structure")

	errInvalidPacketAuthenticator = errors.New("invalid authenticator")
)

const (
	maxNumRetries = 1
)

type filterContext struct {
	epoch          uint64
	alo, amid, ahi float64
	alolo, ahihi   float64
	navg           float64
}

var (
	filters   = make(map[string]filterContext)
	filtersMu = sync.Mutex{}
)

func combine(lo, mid, hi time.Duration, trust float64) (offset time.Duration, weight float64) {
	offset = mid
	weight = 0.001 + trust*2.0/timemath.Seconds(hi-lo)
	if weight < 1.0 {
		weight = 1.0
	}
	return
}

func filter(log *zap.Logger, reference string, cTxTime, sRxTime, sTxTime, cRxTime time.Time) (
	offset time.Duration, weight float64) {

	// Based on Ntimed by Poul-Henning Kamp, https://github.com/bsdphk/Ntimed

	filtersMu.Lock()
	f := filters[reference]

	lo := timemath.Seconds(cTxTime.Sub(sRxTime))
	hi := timemath.Seconds(cRxTime.Sub(sTxTime))
	mid := (lo + hi) / 2

	if f.epoch != timebase.Epoch() {
		f.epoch = timebase.Epoch()
		f.alo = 0.0
		f.amid = 0.0
		f.ahi = 0.0
		f.alolo = 0.0
		f.ahihi = 0.0
		f.navg = 0.0
	}

	const (
		filterAverage   = 20.0
		filterThreshold = 3.0
	)

	if f.navg < filterAverage {
		f.navg += 1.0
	}

	var loNoise, hiNoise float64
	if f.navg > 2.0 {
		loNoise = math.Sqrt(f.alolo - f.alo*f.alo)
		hiNoise = math.Sqrt(f.ahihi - f.ahi*f.ahi)
	}

	loLim := f.alo - loNoise*filterThreshold
	hiLim := f.ahi + hiNoise*filterThreshold

	var branch int
	failLo := lo < loLim
	failHi := hi > hiLim
	if failLo && failHi {
		branch = 1
	} else if f.navg > 3.0 && failLo {
		mid = f.amid + (hi - f.ahi)
		branch = 2
	} else if f.navg > 3.0 && failHi {
		mid = f.amid + (lo - f.alo)
		branch = 3
	} else {
		branch = 4
	}

	r := f.navg
	if f.navg > 2.0 && branch != 4 {
		r *= r
	}

	f.alo += (lo - f.alo) / r
	f.amid += (mid - f.amid) / r
	f.ahi += (hi - f.ahi) / r
	f.alolo += (lo*lo - f.alolo) / r
	f.ahihi += (hi*hi - f.ahihi) / r

	filters[reference] = f
	filtersMu.Unlock()

	trust := 1.0

	offset, weight = combine(timemath.Duration(lo), timemath.Duration(mid), timemath.Duration(hi), trust)

	log.Debug("filtered response",
		zap.String("from", reference),
		zap.Int("branch", branch),
		zap.Float64("lo [s]", lo),
		zap.Float64("mid [s]", mid),
		zap.Float64("hi [s]", hi),
		zap.Float64("loLim [s]", loLim),
		zap.Float64("amid [s]", f.amid),
		zap.Float64("hiLim [s]", hiLim),
		zap.Float64("offset [s]", timemath.Seconds(offset)),
		zap.Float64("weight", weight),
	)

	return timemath.Inv(offset), weight
}

type IPClient struct {
	InterleavedMode bool
	Auth            struct {
		Enabled      bool
		NTSKEFetcher ntske.Fetcher
	}
	prev struct {
		reference string
		cTxTime   ntp.Time64
		cRxTime   ntp.Time64
		sRxTime   ntp.Time64
	}
}

type ipClientMetrics struct {
	reqsSent                 prometheus.Counter
	reqsSentInterleaved      prometheus.Counter
	pktsReceived             prometheus.Counter
	pktsAuthenticated        prometheus.Counter
	respsAccepted            prometheus.Counter
	respsAcceptedInterleaved prometheus.Counter
}

func newIPClientMetrics() *ipClientMetrics {
	return &ipClientMetrics{
		reqsSent: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientReqsSentN,
			Help: metrics.IPClientReqsSentH,
		}),
		reqsSentInterleaved: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientReqsSentInterleavedN,
			Help: metrics.IPClientReqsSentInterleavedH,
		}),
		pktsReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientPktsReceivedN,
			Help: metrics.IPClientPktsReceivedH,
		}),
		pktsAuthenticated: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientPktsAuthenticatedN,
			Help: metrics.IPClientPktsAuthenticatedH,
		}),
		respsAccepted: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientRespsAcceptedN,
			Help: metrics.IPClientRespsAcceptedH,
		}),
		respsAcceptedInterleaved: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientRespsAcceptedInterleavedN,
			Help: metrics.IPClientRespsAcceptedInterleavedH,
		}),
	}
}

func compareAddrs(x, y netip.Addr) int {
	if x.Is4In6() {
		x = netip.AddrFrom4(x.As4())
	}
	if y.Is4In6() {
		y = netip.AddrFrom4(y.As4())
	}
	return x.Compare(y)
}

func (c *IPClient) ResetInterleavedMode() {
	c.prev.reference = ""
}

var (
	errNoPaths = errors.New("failed to measure clock offset: no paths")

	ipMetrics atomic.Pointer[ipClientMetrics]
)

func init() {
	ipMetrics.Store(newIPClientMetrics())
}

func (c *IPClient) measureClockOffsetIP(ctx context.Context, log *zap.Logger, mtrcs *ipClientMetrics, hg *hdrhistogram.Histogram,
	localAddr, remoteAddr *net.UDPAddr) (
	offset time.Duration, weight float64, err error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localAddr.IP})
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
	err = udp.EnableTimestamping(conn, localAddr.Zone)
	if err != nil {
		log.Error("failed to enable timestamping", zap.Error(err))
	}
	err = udp.SetDSCP(conn, config.DSCP)
	if err != nil {
		log.Info("failed to set DSCP", zap.Error(err))
	}

	var ntskeData ntske.Data
	if c.Auth.Enabled {
		ntskeData, err = c.Auth.NTSKEFetcher.FetchData()
		if err != nil {
			log.Info("failed to fetch key exchange data", zap.Error(err))
			return offset, weight, err
		}
		remoteAddr.IP = net.ParseIP(ntskeData.Server)
		if remoteAddr.IP.To4() != nil {
			remoteAddr.IP = remoteAddr.IP.To4()
		}
		remoteAddr.Port = int(ntskeData.Port)
	}

	buf := make([]byte, ntp.PacketLen)

	reference := remoteAddr.String()
	cTxTime0 := timebase.Now()
	interleaved := false

	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	if c.InterleavedMode && reference == c.prev.reference &&
		cTxTime0.Sub(ntp.TimeFromTime64(c.prev.cTxTime)) <= time.Second {
		interleaved = true
		ntpreq.OriginTime = c.prev.sRxTime
		ntpreq.ReceiveTime = c.prev.cRxTime
		ntpreq.TransmitTime = c.prev.cTxTime
	} else {
		ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime0)
	}

	ntp.EncodePacket(&buf, &ntpreq)

	var requestID []byte
	var ntsreq nts.NTSPacket
	if c.Auth.Enabled {
		ntsreq, requestID = nts.NewPacket(buf, ntskeData)
		nts.EncodePacket(&buf, &ntsreq)
	}

	n, err := conn.WriteToUDPAddrPort(buf, remoteAddr.AddrPort())
	if err != nil {
		return offset, weight, err
	}
	if n != len(buf) {
		return offset, weight, errWrite
	}
	cTxTime1, id, err := udp.ReadTXTimestamp(conn)
	if err != nil || id != 0 {
		cTxTime1 = timebase.Now()
		log.Error("failed to read packet tx timestamp", zap.Error(err))
	}
	mtrcs.reqsSent.Inc()
	if interleaved {
		mtrcs.reqsSentInterleaved.Inc()
	}

	numRetries := 0
	oob := make([]byte, udp.TimestampLen())
	for {
		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				log.Info("failed to read packet", zap.Error(err))
				numRetries++
				continue
			}
			return offset, weight, err
		}
		if flags != 0 {
			err = errUnexpectedPacketFlags
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				log.Info("failed to read packet", zap.Int("flags", flags))
				numRetries++
				continue
			}
			return offset, weight, err
		}
		oob = oob[:oobn]
		cRxTime, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			cRxTime = timebase.Now()
			log.Error("failed to read packet rx timestamp", zap.Error(err))
		}
		buf = buf[:n]
		mtrcs.pktsReceived.Inc()

		if compareAddrs(srcAddr.Addr(), remoteAddr.AddrPort().Addr()) != 0 {
			err = errUnexpectedPacketSource
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				log.Info("received packet from unexpected source")
				numRetries++
				continue
			}
			return offset, weight, err
		}

		var ntpresp ntp.Packet
		err = ntp.DecodePacket(&ntpresp, buf)
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				log.Info("failed to decode packet payload", zap.Error(err))
				numRetries++
				continue
			}
			return offset, weight, err
		}

		authenticated := false
		var ntsresp nts.NTSPacket
		if c.Auth.Enabled {
			err = nts.DecodePacket(&ntsresp, buf, ntskeData.S2cKey)
			if err != nil {
				if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
					log.Info("failed to decode and authenticate NTS packet", zap.Error(err))
					numRetries++
					continue
				}
				return offset, weight, err
			}

			err = nts.ProcessResponse(&c.Auth.NTSKEFetcher, &ntsresp, requestID)
			if err != nil {
				if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
					log.Info("failed to process NTS packet", zap.Error(err))
					numRetries++
					continue
				}
				return offset, weight, err
			}

			authenticated = true
			mtrcs.pktsAuthenticated.Inc()
		}

		interleaved = false
		if c.InterleavedMode && ntpresp.OriginTime == c.prev.cRxTime {
			interleaved = true
		} else if ntpresp.OriginTime != ntpreq.TransmitTime {
			err = errUnexpectedPacket
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				log.Info("received packet with unexpected type or structure")
				numRetries++
				continue
			}
			return offset, weight, err
		}

		err = ntp.ValidateResponseMetadata(&ntpresp)
		if err != nil {
			return offset, weight, err
		}

		log.Debug("received response",
			zap.Time("at", cRxTime),
			zap.String("from", reference),
			zap.Bool("auth", authenticated),
			zap.Object("data", ntp.PacketMarshaler{Pkt: &ntpresp}),
		)

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

		err = ntp.ValidateResponseTimestamps(t0, t1, t1, t3)
		if err != nil {
			return offset, weight, err
		}

		off := ntp.ClockOffset(t0, t1, t2, t3)
		rtd := ntp.RoundTripDelay(t0, t1, t2, t3)

		mtrcs.respsAccepted.Inc()
		if interleaved {
			mtrcs.respsAcceptedInterleaved.Inc()
		}
		log.Debug("evaluated response",
			zap.String("from", reference),
			zap.Bool("interleaved", interleaved),
			zap.Duration("clock offset", off),
			zap.Duration("round trip delay", rtd),
		)

		if c.InterleavedMode {
			c.prev.reference = reference
			c.prev.cTxTime = ntp.Time64FromTime(cTxTime1)
			c.prev.cRxTime = ntp.Time64FromTime(cRxTime)
			c.prev.sRxTime = ntpresp.ReceiveTime
		}

		// offset, weight = off, 1000.0

		offset, weight = filter(log, reference, t0, t1, t2, t3)

		err = hg.RecordValue(rtd.Microseconds())
		if err != nil {
			log.Error("Failed to record histogram value", zap.Error(err))
		}

		break
	}

	return offset, weight, nil
}

func RunIPBenchmark(localAddr, remoteAddr *net.UDPAddr, authMode, ntskeServer string, log *zap.Logger) {
	// const numClientGoroutine = 8
	// const numRequestPerClient = 10000
	const numClientGoroutine = 500
	const numRequestPerClient = 1_000
	var mu sync.Mutex
	sg := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numClientGoroutine)
	var err error

	if err != nil {
		log.Fatal("failed to measure clock offset", zap.Stringer("to", remoteAddr), zap.Error(err))
	}
	mtrcs := ipMetrics.Load()


	for i := numClientGoroutine; i > 0; i-- {
		go func() {
			hg := hdrhistogram.New(1, 50000, 5)
			ctx := context.Background()

			c := &IPClient{
				InterleavedMode: true,
			}

			if authMode == "nts" {
				ntskeHost, ntskePort, err := net.SplitHostPort(ntskeServer)
				if err != nil {
					log.Fatal("failed to split NTS-KE host and port", zap.Error(err))
				}
				c.Auth.Enabled = true
				c.Auth.NTSKEFetcher.TLSConfig = tls.Config{
					InsecureSkipVerify: true,
					ServerName:         ntskeHost,
					MinVersion:         tls.VersionTLS13,
				}
				c.Auth.NTSKEFetcher.Port = ntskePort
				c.Auth.NTSKEFetcher.Log = log
			}
			defer wg.Done()
			<-sg
			for j := numRequestPerClient; j > 0; j-- {

				_, _, err = c.measureClockOffsetIP(ctx, log, mtrcs, hg, localAddr, remoteAddr)
				if err != nil {
					log.Info("measure failed", zap.Error(err))
				}

			}
			mu.Lock()
			defer mu.Unlock()
			hg.PercentilesPrint(os.Stdout, 1, 1.0)
		}()
	}
	t0 := time.Now()
	close(sg)
	wg.Wait()
	log.Info(time.Since(t0).String())
}
