package server

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/core/timebase"
	"example.com/scion-time/net/csptp"
	"example.com/scion-time/net/udp"
)

const (
	csptpContextCap = 8
	csptpClientCap  = 1 << 20
)

//lint:ignore U1000 work in progress
type csptpContext struct {
	conn       *net.UDPConn
	srcPort    uint16
	rxTime     time.Time
	sequenceID uint16
	correction int64
}

//lint:ignore U1000 work in progress
type csptpClient struct {
	key   netip.Addr
	ctxts [csptpContextCap]csptpContext
	len   int
	qval  time.Time
	qidx  int
}

type csptpClientQueue []*csptpClient

var (
	csptpClients  = make(map[netip.Addr]*csptpClient)
	csptpClientsQ = make(csptpClientQueue, 0, csptpClientCap)

	csptpMu sync.Mutex
)

func (q csptpClientQueue) Len() int { return len(q) }

func (q csptpClientQueue) Less(i, j int) bool {
	return q[i].qval.Before(q[j].qval)
}

func (q csptpClientQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].qidx = i
	q[j].qidx = j
}

func (q *csptpClientQueue) Push(x any) {
	c := x.(*csptpClient)
	c.qidx = len(*q)
	*q = append(*q, c)
}

func (q *csptpClientQueue) Pop() any {
	n := len(*q)
	c := (*q)[n-1]
	(*q)[n-1] = nil
	*q = (*q)[0 : n-1]
	return c
}

func runCSPTPServerIP(ctx context.Context, log *slog.Logger,
	conn *net.UDPConn, localHostIface string, localHostPort int, dscp uint8) {
	err := udp.EnableTimestamping(conn, localHostIface)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelError, "failed to enable timestamping", slog.Any("error", err))
	}
	err = udp.SetDSCP(conn, dscp)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to set DSCP", slog.Any("error", err))
	}

	var eConn, gConn *net.UDPConn
	var eTxID, gTxID uint32

	buf := make([]byte, csptp.MaxMessageLength)
	oob := make([]byte, udp.TimestampLen())
	for {
		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			log.LogAttrs(ctx, slog.LevelError, "failed to read packet", slog.Any("error", err))
			continue
		}
		if flags != 0 {
			log.LogAttrs(ctx, slog.LevelError, "failed to read packet", slog.Int("flags", flags))
			continue
		}
		oob = oob[:oobn]
		rxt, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			oob = oob[:0]
			rxt = timebase.Now()
			log.LogAttrs(ctx, slog.LevelError, "failed to read packet rx timestamp", slog.Any("error", err))
		}
		buf = buf[:n]

		if len(buf) < csptp.MinMessageLength {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload: unexpected structure")
			continue
		}

		var reqmsg csptp.Message
		err = csptp.DecodeMessage(&reqmsg, buf[:csptp.MinMessageLength])
		if err != nil {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
			continue
		}

		if len(buf) != int(reqmsg.MessageLength) {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected message length")
			continue
		}

		if reqmsg.SdoIDMessageType == csptp.MessageTypeSync && localHostPort == csptp.EventPortIP {
			if len(buf)-csptp.MinMessageLength != 0 {
				log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected Sync message length")
				continue
			}

			log.LogAttrs(ctx, slog.LevelDebug, "received request",
				slog.Time("at", rxt),
				slog.String("from", srcAddr.String()),
				slog.Any("reqmsg", &reqmsg),
			)
		} else if reqmsg.SdoIDMessageType == csptp.MessageTypeFollowUp && localHostPort == csptp.GeneralPortIP {
			var reqtlv csptp.RequestTLV
			err = csptp.DecodeRequestTLV(&reqtlv, buf[csptp.MinMessageLength:])
			if err != nil {
				log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
				continue
			}
			if reqtlv.Type != csptp.TLVTypeOrganizationExtension ||
				reqtlv.OrganizationID[0] != csptp.OrganizationIDMeinberg0 ||
				reqtlv.OrganizationID[1] != csptp.OrganizationIDMeinberg1 ||
				reqtlv.OrganizationID[2] != csptp.OrganizationIDMeinberg2 ||
				reqtlv.OrganizationSubType[0] != csptp.OrganizationSubTypeRequest0 ||
				reqtlv.OrganizationSubType[1] != csptp.OrganizationSubTypeRequest1 ||
				reqtlv.OrganizationSubType[2] != csptp.OrganizationSubTypeRequest2 {
				log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected Follow Up message")
				continue
			}
			if len(buf)-csptp.MinMessageLength != csptp.EncodedRequestTLVLength(&reqtlv) {
				log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected Follow Up message length")
				continue
			}

			log.LogAttrs(ctx, slog.LevelDebug, "received request",
				slog.Time("at", rxt),
				slog.String("from", srcAddr.String()),
				slog.Any("reqmsg", &reqmsg),
				slog.Any("reqtlv", &reqtlv),
			)
		} else {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected message")
			continue
		}

		var (
			sequenceID       uint16
			sequenceComplete bool
			syncSrcPort      uint16
			followUpSrcPort  uint16
		)

		csptpMu.Lock()
		// maintain CSPTP client data structure
		_ = len(csptpClients)
		_ = len(csptpClientsQ)
		csptpMu.Unlock()

		if sequenceComplete {
			var msg csptp.Message
			var resptlv csptp.ResponseTLV

			buf = buf[:cap(buf)]

			msg = csptp.Message{
				SdoIDMessageType:    csptp.MessageTypeSync,
				PTPVersion:          csptp.PTPVersion,
				MessageLength:       csptp.MinMessageLength,
				DomainNumber:        csptp.DomainNumber,
				MinorSdoID:          csptp.MinorSdoID,
				FlagField:           csptp.FlagTwoStep | csptp.FlagUnicast,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: csptp.PortID{
					ClockID: 1,
					Port:    1,
				},
				SequenceID:         sequenceID,
				ControlField:       csptp.ControlSync,
				LogMessageInterval: csptp.LogMessageInterval,
				Timestamp:          csptp.Timestamp{},
			}

			buf = buf[:msg.MessageLength]
			csptp.EncodeMessage(buf, &msg)

			n, err = eConn.WriteToUDPAddrPort(
				buf, netip.AddrPortFrom(srcAddr.Addr(), syncSrcPort))
			if err != nil || n != len(buf) {
				log.LogAttrs(ctx, slog.LevelError, "failed to write packet", slog.Any("error", err))
				continue
			}
			cTxTime0, id, err := udp.ReadTXTimestamp(eConn)
			if err != nil {
				cTxTime0 = timebase.Now()
				log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp",
					slog.Any("error", err))
			} else if id != eTxID {
				cTxTime0 = timebase.Now()
				log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp",
					slog.Uint64("id", uint64(id)), slog.Uint64("expected", uint64(eTxID)))
				eTxID = id + 1
			} else {
				eTxID++
			}
			_ = cTxTime0

			buf = buf[:cap(buf)]

			msg = csptp.Message{
				SdoIDMessageType:    csptp.MessageTypeFollowUp,
				PTPVersion:          csptp.PTPVersion,
				MessageLength:       csptp.MinMessageLength,
				DomainNumber:        csptp.DomainNumber,
				MinorSdoID:          csptp.MinorSdoID,
				FlagField:           csptp.FlagUnicast,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: csptp.PortID{
					ClockID: 1,
					Port:    1,
				},
				SequenceID:         sequenceID,
				ControlField:       csptp.ControlFollowUp,
				LogMessageInterval: csptp.LogMessageInterval,
				Timestamp:          csptp.Timestamp{}, /* TODO */
			}
			resptlv = csptp.ResponseTLV{
				Type:   csptp.TLVTypeOrganizationExtension,
				Length: 0,
				OrganizationID: [3]uint8{
					csptp.OrganizationIDMeinberg0,
					csptp.OrganizationIDMeinberg1,
					csptp.OrganizationIDMeinberg2},
				OrganizationSubType: [3]uint8{
					csptp.OrganizationSubTypeResponse0,
					csptp.OrganizationSubTypeResponse1,
					csptp.OrganizationSubTypeResponse2},
				FlagField:               csptp.TLVFlagServerStateDS,
				Error:                   0,
				RequestIngressTimestamp: csptp.Timestamp{}, /* TODO */
				RequestCorrectionField:  0,
				UTCOffset:               0,
				ServerStateDS: csptp.ServerStateDS{
					GMPriority1:     0, /* TODO */
					GMClockClass:    0, /* TODO */
					GMClockAccuracy: 0, /* TODO */
					GMClockVariance: 0, /* TODO */
					GMPriority2:     0, /* TODO */
					GMClockID:       0, /* TODO */
					StepsRemoved:    0, /* TODO */
					TimeSource:      0, /* TODO */
					Reserved:        0,
				},
			}
			msg.MessageLength += uint16(csptp.EncodedResponseTLVLength(&resptlv))
			resptlv.Length = uint16(csptp.EncodedResponseTLVLength(&resptlv))

			buf = buf[:msg.MessageLength]
			csptp.EncodeMessage(buf[:csptp.MinMessageLength], &msg)
			csptp.EncodeResponseTLV(buf[csptp.MinMessageLength:], &resptlv)

			n, err = gConn.WriteToUDPAddrPort(
				buf, netip.AddrPortFrom(srcAddr.Addr(), followUpSrcPort))
			if err != nil || n != len(buf) {
				log.LogAttrs(ctx, slog.LevelError, "failed to write packet", slog.Any("error", err))
				continue
			}
			cTxTime1, id, err := udp.ReadTXTimestamp(gConn)
			if err != nil {
				cTxTime1 = timebase.Now()
				log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp",
					slog.Any("error", err))
			} else if id != gTxID {
				cTxTime1 = timebase.Now()
				log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp",
					slog.Uint64("id", uint64(id)), slog.Uint64("expected", uint64(gTxID)))
				gTxID = id + 1
			} else {
				gTxID++
			}
			_ = cTxTime1
		}
	}
}

func StartCSPTPServerIP(ctx context.Context, log *slog.Logger,
	localHost *net.UDPAddr, dscp uint8) {
	log.LogAttrs(ctx, slog.LevelInfo, "CSPTP server listening via IP",
		slog.Any("local host", localHost.IP),
	)

	if localHost.Port != 0 {
		logbase.FatalContext(ctx, log, "unexpected listener port",
			slog.Int("port", localHost.Port))
	}

	lc := net.ListenConfig{
		Control: udp.SetsockoptReuseAddrPort,
	}
	for _, localHostPort := range []int{csptp.EventPortIP, csptp.GeneralPortIP} {
		address := net.JoinHostPort(localHost.IP.String(), strconv.Itoa(localHostPort))
		for range ipServerNumGoroutine {
			conn, err := lc.ListenPacket(ctx, "udp", address)
			if err != nil {
				logbase.FatalContext(ctx, log, "failed to listen for packets", slog.Any("error", err))
			}
			go runCSPTPServerIP(ctx, log, conn.(*net.UDPConn), localHost.Zone, localHostPort, dscp)
		}
	}
}
