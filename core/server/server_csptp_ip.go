package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"time"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/core/timebase"
	"example.com/scion-time/net/csptp"
	"example.com/scion-time/net/udp"
)

type messageHandler func(ctx context.Context, log *slog.Logger,
	buf *[]byte, srcAddr netip.AddrPort, rxt time.Time) error

var (
	errUnexpectedMessage = errors.New("failed to read message: unexpected type or structure")
)

func handleMessage(ctx context.Context, log *slog.Logger,
	buf *[]byte, srcAddr netip.AddrPort, rxt time.Time) error {
	var err error

	if len(*buf) < csptp.MinMessageLength {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload: unexpected structure")
		return errUnexpectedMessage
	}

	var reqmsg csptp.Message
	err = csptp.DecodeMessage(&reqmsg, (*buf)[:csptp.MinMessageLength])
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
		return err
	}

	if len(*buf) < int(reqmsg.MessageLength) {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected message length")
		return errUnexpectedMessage
	}

	if reqmsg.SdoIDMessageType == csptp.MessageTypeSync {
		if len(*buf)-csptp.MinMessageLength != 0 {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected Sync message length")
			return errUnexpectedMessage
		}

		log.LogAttrs(ctx, slog.LevelDebug, "received request",
			slog.Time("at", rxt),
			slog.String("from", srcAddr.String()),
			slog.Any("reqmsg", &reqmsg),
		)

		// Handle Sync Request

		// Encode response
		respmsg := csptp.Message{
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
			SequenceID:         reqmsg.SequenceID,
			ControlField:       csptp.ControlSync,
			LogMessageInterval: csptp.LogMessageInterval,
			Timestamp:          csptp.Timestamp{},
		}

		*buf = (*buf)[:respmsg.MessageLength]
		csptp.EncodeMessage(*buf, &respmsg)
	} else if reqmsg.SdoIDMessageType == csptp.MessageTypeFollowUp {
		var reqtlv csptp.RequestTLV
		err = csptp.DecodeRequestTLV(&reqtlv, (*buf)[csptp.MinMessageLength:])
		if err != nil {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
			return err
		}
		if reqtlv.Type != csptp.TLVTypeOrganizationExtension ||
			reqtlv.OrganizationID[0] != csptp.OrganizationIDMeinberg0 ||
			reqtlv.OrganizationID[1] != csptp.OrganizationIDMeinberg1 ||
			reqtlv.OrganizationID[2] != csptp.OrganizationIDMeinberg2 ||
			reqtlv.OrganizationSubType[0] != csptp.OrganizationSubTypeRequest0 ||
			reqtlv.OrganizationSubType[1] != csptp.OrganizationSubTypeRequest1 ||
			reqtlv.OrganizationSubType[2] != csptp.OrganizationSubTypeRequest2 {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected Follow Up message")
			return errUnexpectedMessage
		}
		if len(*buf)-csptp.MinMessageLength != csptp.EncodedRequestTLVLength(&reqtlv) {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected Follow Up message length")
			return errUnexpectedMessage
		}

		log.LogAttrs(ctx, slog.LevelDebug, "received request",
			slog.Time("at", rxt),
			slog.String("from", srcAddr.String()),
			slog.Any("reqmsg", &reqmsg),
			slog.Any("reqtlv", &reqtlv),
		)

		// Handle Follow Up Request

		respmsg := csptp.Message{
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
			SequenceID:         reqmsg.SequenceID,
			ControlField:       csptp.ControlFollowUp,
			LogMessageInterval: csptp.LogMessageInterval,
			Timestamp:          csptp.Timestamp{}, /* TODO */
		}
		resptlv := csptp.ResponseTLV{
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
		respmsg.MessageLength += uint16(csptp.EncodedResponseTLVLength(&resptlv))
		resptlv.Length = uint16(csptp.EncodedResponseTLVLength(&resptlv))

		*buf = (*buf)[:respmsg.MessageLength]
		csptp.EncodeMessage((*buf)[:csptp.MinMessageLength], &respmsg)
		csptp.EncodeResponseTLV((*buf)[csptp.MinMessageLength:], &resptlv)
	} else {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to validate packet payload: unexpected message")
		return errUnexpectedMessage
	}

	return nil
}

func runCSPTPServerIP(ctx context.Context, log *slog.Logger,
	conn *net.UDPConn, iface string, dscp uint8, h messageHandler) {
	err := udp.EnableTimestamping(conn, iface)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelError, "failed to enable timestamping", slog.Any("error", err))
	}
	err = udp.SetDSCP(conn, dscp)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to set DSCP", slog.Any("error", err))
	}
	var txID uint32
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

		err = handleMessage(ctx, log, &buf, srcAddr, rxt)
		if err != nil {
			continue
		}

		n, err = conn.WriteToUDPAddrPort(buf, srcAddr)
		if err != nil || n != len(buf) {
			log.LogAttrs(ctx, slog.LevelError, "failed to write packet", slog.Any("error", err))
			continue
		}
		txt, id, err := udp.ReadTXTimestamp(conn)
		if err != nil {
			txt = timebase.Now()
			log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp",
				slog.Any("error", err))
		} else if id != txID {
			txt = timebase.Now()
			log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp",
				slog.Uint64("id", uint64(id)), slog.Uint64("expected", uint64(txID)))
			txID = id + 1
		} else {
			txID++
		}

		// Update tx timestamp
		_ = txt
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
	for _, port := range []int{csptp.EventPortIP, csptp.GeneralPortIP} {
		address := net.JoinHostPort(localHost.IP.String(), strconv.Itoa(port))
		for range ipServerNumGoroutine {
			conn, err := lc.ListenPacket(ctx, "udp", address)
			if err != nil {
				logbase.FatalContext(ctx, log, "failed to listen for packets", slog.Any("error", err))
			}
			go runCSPTPServerIP(ctx, log, conn.(*net.UDPConn), localHost.Zone, dscp, handleMessage)
		}
	}
}
