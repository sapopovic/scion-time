package server

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/google/gopacket"
	"github.com/libp2p/go-reuseport"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"go.uber.org/zap"

	"example.com/scion-time/base/metrics"

	"example.com/scion-time/core/config"
	"example.com/scion-time/core/timebase"

	"example.com/scion-time/net/gopacketntp"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/udp"
)

const (
	ipServerNumGoroutine = 8
)

type ipServerMetrics struct {
	pktsReceived prometheus.Counter
	reqsAccepted prometheus.Counter
	reqsServed   prometheus.Counter
}

func newIPServerMetrics() *ipServerMetrics {
	return &ipServerMetrics{
		pktsReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPServerPktsReceivedN,
			Help: metrics.IPServerPktsReceivedH,
		}),
		reqsAccepted: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPServerReqsAcceptedN,
			Help: metrics.IPServerReqsAcceptedH,
		}),
		reqsServed: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPServerReqsServedN,
			Help: metrics.IPServerReqsServedH,
		}),
	}
}

func runIPServer(log *zap.Logger, mtrcs *ipServerMetrics, conn *net.UDPConn, iface string, provider *ntske.Provider) {
	defer conn.Close()
	err := udp.EnableTimestamping(conn, iface)
	if err != nil {
		log.Error("failed to enable timestamping", zap.Error(err))
	}
	err = udp.SetDSCP(conn, config.DSCP)
	if err != nil {
		log.Info("failed to set DSCP", zap.Error(err))
	}

	var txID uint32
	buf := make([]byte, 2048)
	oob := make([]byte, udp.TimestampLen())
	for {
		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			log.Error("failed to read packet", zap.Error(err))
			continue
		}
		if flags != 0 {
			log.Error("failed to read packet", zap.Int("flags", flags))
			continue
		}
		oob = oob[:oobn]
		rxt, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			oob = oob[:0]
			rxt = timebase.Now()
			log.Error("failed to read packet rx timestamp", zap.Error(err))
		}
		buf = buf[:n]
		mtrcs.pktsReceived.Inc()

		var ntpreq gopacketntp.Packet
		parser := gopacket.NewDecodingLayerParser(gopacketntp.LayerTypeNTS, &ntpreq)
		parser.IgnoreUnsupported = true
		decoded := make([]gopacket.LayerType, 1)
		err = parser.DecodeLayers(buf, &decoded)
		if err != nil {
			log.Info("failed to decode packet payload", zap.Error(err))
			continue
		}

		var authenticated bool
		var serverCookie ntske.ServerCookie
		if len(buf) > gopacketntp.PacketLen {
			if ntpreq.Cookies == nil || len(ntpreq.Cookies) < 1 {
				log.Info("failed to extract cookie", zap.Error(err))
				continue
			}

			var encryptedCookie ntske.EncryptedServerCookie
			err = encryptedCookie.Decode(ntpreq.Cookies[0].Cookie)
			if err != nil {
				log.Info("failed to decode cookie", zap.Error(err))
				continue
			}

			key, ok := provider.Get(int(encryptedCookie.ID))
			if !ok {
				log.Info("failed to get key", zap.Error(err))
				continue
			}

			serverCookie, err = encryptedCookie.Decrypt(key.Value)
			if err != nil {
				log.Info("failed to decrypt cookie", zap.Error(err))
				continue
			}

			err = ntpreq.Authenticate(serverCookie.C2S)
			if err != nil {
				log.Info("failed to authenticate packet", zap.Error(err))
				continue
			}
			authenticated = true
		}

		err = gopacketntp.ValidateRequest(&ntpreq, srcAddr.Port())
		if err != nil {
			log.Info("failed to validate packet payload", zap.Error(err))
			continue
		}

		clientID := srcAddr.Addr().String()

		mtrcs.reqsAccepted.Inc()
		log.Debug("received request",
			zap.Time("at", rxt),
			zap.String("from", clientID),
			zap.Bool("ntsauth", authenticated),
			zap.Object("data", gopacketntp.PacketMarshaler{Pkt: &ntpreq}),
		)

		var txt0 time.Time
		var ntpresp gopacketntp.Packet
		handleRequestGopacket(clientID, &ntpreq, &rxt, &txt0, &ntpresp)

		if authenticated {
			var cookies [][]byte
			key := provider.Current()
			addedCookie := false
			for i := 0; i < len(ntpreq.Cookies)+len(ntpreq.CookiePlaceholders); i++ {
				encryptedCookie, err := serverCookie.EncryptWithNonce(key.Value, key.ID)
				if err != nil {
					log.Info("failed to encrypt cookie", zap.Error(err))
					continue
				}
				cookie := encryptedCookie.Encode()
				cookies = append(cookies, cookie)
				addedCookie = true
			}
			if !addedCookie {
				log.Info("failed to add at least one cookie")
				continue
			}

			ntpresp.InitNTSResponsePacket(cookies, serverCookie.S2C, ntpreq.UniqueID.ID)
		}

		buffer := gopacket.NewSerializeBuffer()
		options := gopacket.SerializeOptions{
			ComputeChecksums: true,
			FixLengths:       true,
		}

		err = ntpresp.SerializeTo(buffer, options)
		if err != nil {
			panic(err)
		}
		buffer.PushLayer(ntpresp.LayerType())

		n, err = conn.WriteToUDPAddrPort(buffer.Bytes(), srcAddr)
		if err != nil || n != len(buf) {
			log.Error("failed to write packet", zap.Error(err))
			continue
		}
		txt1, id, err := udp.ReadTXTimestamp(conn)
		if err != nil {
			txt1 = txt0
			log.Error("failed to read packet tx timestamp", zap.Error(err))
		} else if id != txID {
			txt1 = txt0
			log.Error("failed to read packet tx timestamp", zap.Uint32("id", id), zap.Uint32("expected", txID))
			txID = id + 1
		} else {
			txID++
		}
		updateTXTimestamp(clientID, rxt, &txt1)

		mtrcs.reqsServed.Inc()
	}
}

func StartIPServer(ctx context.Context, log *zap.Logger,
	localHost *net.UDPAddr, provider *ntske.Provider) {
	log.Info("server listening via IP",
		zap.Stringer("ip", localHost.IP),
		zap.Int("port", localHost.Port),
	)

	mtrcs := newIPServerMetrics()

	if ipServerNumGoroutine == 1 {
		conn, err := net.ListenUDP("udp", localHost)
		if err != nil {
			log.Fatal("failed to listen for packets", zap.Error(err))
		}
		go runIPServer(log, mtrcs, conn, localHost.Zone, provider)
	} else {
		for i := ipServerNumGoroutine; i > 0; i-- {
			conn, err := reuseport.ListenPacket("udp",
				net.JoinHostPort(localHost.IP.String(), strconv.Itoa(localHost.Port)))
			if err != nil {
				log.Fatal("failed to listen for packets", zap.Error(err))
			}
			go runIPServer(log, mtrcs, conn.(*net.UDPConn), localHost.Zone, provider)
		}
	}
}
