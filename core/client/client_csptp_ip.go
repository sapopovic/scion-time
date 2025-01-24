package client

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"example.com/scion-time/core/timebase"
	"example.com/scion-time/net/csptp"
	"example.com/scion-time/net/udp"
)

//lint:ignore U1000 work in progress
type CSPTPClientIP struct {
	Log        *slog.Logger
	DSCP       uint8
	sequenceID uint16
}

func (c *CSPTPClientIP) MeasureClockOffset(ctx context.Context, localAddr, remoteAddr netip.Addr) (
	timestamp time.Time, offset time.Duration, err error) {
	var lc net.ListenConfig
	pconn, err := lc.ListenPacket(ctx, "udp", netip.AddrPortFrom(localAddr, 0).String())
	if err != nil {
		return time.Time{}, 0, err
	}
	conn := pconn.(*net.UDPConn)
	defer conn.Close()
	deadline, deadlineIsSet := ctx.Deadline()
	if deadlineIsSet {
		err = conn.SetDeadline(deadline)
		if err != nil {
			return time.Time{}, 0, err
		}
	}
	err = udp.EnableTimestamping(conn, localAddr.Zone())
	if err != nil {
		c.Log.LogAttrs(ctx, slog.LevelError, "failed to enable timestamping", slog.Any("error", err))
	}
	err = udp.SetDSCP(conn, c.DSCP)
	if err != nil {
		c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to set DSCP", slog.Any("error", err))
	}

	buf := make([]byte, csptp.MaxMessageLength)
	var n int
	var msg csptp.Message
	var reqtlv csptp.RequestTLV

	cTxTime0 := timebase.Now()

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
			ClockID: 0,
			Port:    1,
		},
		SequenceID:         0,
		ControlField:       csptp.ControlSync,
		LogMessageInterval: 0,
		Timestamp:          csptp.Timestamp{},
	}

	buf = buf[:msg.MessageLength]
	csptp.EncodeMessage(buf, &msg)

	n, err = conn.WriteToUDPAddrPort(buf, netip.AddrPortFrom(remoteAddr, csptp.EventPortIP))
	if err != nil {
		return time.Time{}, 0, err
	}
	if n != len(buf) {
		return time.Time{}, 0, errWrite
	}
	cTxTime1, id, err := udp.ReadTXTimestamp(conn)
	if err != nil || id != 0 {
		cTxTime1 = timebase.Now()
		c.Log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp", slog.Any("error", err))
	}

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
			ClockID: 0,
			Port:    1,
		},
		SequenceID:         0,
		ControlField:       csptp.ControlFollowUp,
		LogMessageInterval: 0,
		Timestamp:          csptp.Timestamp{},
	}
	reqtlv = csptp.RequestTLV{
		Type:   csptp.TLVTypeOrganizationExtension,
		Length: 0,
		OrganizationID: [3]uint8{
			csptp.OrganizationIDMeinberg0,
			csptp.OrganizationIDMeinberg1,
			csptp.OrganizationIDMeinberg2},
		OrganizationSubType: [3]uint8{
			csptp.OrganizationSubTypeRequest0,
			csptp.OrganizationSubTypeRequest1,
			csptp.OrganizationSubTypeRequest2},
		FlagField: csptp.TLVFlagServerStateDS,
	}
	msg.MessageLength += uint16(csptp.EncodedRequestTLVLength(&reqtlv))
	reqtlv.Length = uint16(csptp.EncodedRequestTLVLength(&reqtlv))

	buf = buf[:msg.MessageLength]
	csptp.EncodeMessage(buf[:csptp.MinMessageLength], &msg)
	csptp.EncodeRequestTLV(buf[csptp.MinMessageLength:], &reqtlv)

	n, err = conn.WriteToUDPAddrPort(buf, netip.AddrPortFrom(remoteAddr, csptp.GeneralPortIP))
	if err != nil {
		return time.Time{}, 0, err
	}
	if n != len(buf) {
		return time.Time{}, 0, errWrite
	}
	cTxTime2, id, err := udp.ReadTXTimestamp(conn)
	if err != nil || id != 0 {
		cTxTime1 = timebase.Now()
		c.Log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp", slog.Any("error", err))
	}

	const maxNumRetries = 1
	numRetries := 0
	oob := make([]byte, udp.TimestampLen())
	for {
		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to read packet", slog.Any("error", err))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}
		if flags != 0 {
			err = errUnexpectedPacketFlags
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to read packet", slog.Int("flags", flags))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}
		oob = oob[:oobn]
		cRxTime0, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			cRxTime0 = timebase.Now()
			c.Log.LogAttrs(ctx, slog.LevelError, "failed to read packet rx timestamp", slog.Any("error", err))
		}
		buf = buf[:n]

		if srcAddr.Compare(netip.AddrPortFrom(remoteAddr, csptp.EventPortIP)) != 0 {
			err = errUnexpectedPacketSource
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "received packet from unexpected source")
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}

		var respmsg0 csptp.Message
		err = csptp.DecodeMessage(&respmsg0, buf)
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}

		if len(buf) != int(respmsg0.MessageLength) {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}

		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, srcAddr, err = conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to read packet", slog.Any("error", err))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}
		if flags != 0 {
			err = errUnexpectedPacketFlags
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to read packet", slog.Int("flags", flags))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}
		oob = oob[:oobn]
		cRxTime1, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			cRxTime1 = timebase.Now()
			c.Log.LogAttrs(ctx, slog.LevelError, "failed to read packet rx timestamp", slog.Any("error", err))
		}
		buf = buf[:n]

		if srcAddr.Compare(netip.AddrPortFrom(remoteAddr, csptp.GeneralPortIP)) != 0 {
			err = errUnexpectedPacketSource
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "received packet from unexpected source")
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}

		var respmsg1 csptp.Message
		err = csptp.DecodeMessage(&respmsg1, buf[:csptp.MinMessageLength])
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}
		var resptlv csptp.ResponseTLV
		err = csptp.DecodeResponseTLV(&resptlv, buf[csptp.MinMessageLength:])
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}

		if len(buf) != int(respmsg1.MessageLength) {
			if numRetries != maxNumRetries && deadlineIsSet && timebase.Now().Before(deadline) {
				c.Log.LogAttrs(ctx, slog.LevelInfo, "failed to decode packet payload", slog.Any("error", err))
				numRetries++
				continue
			}
			return time.Time{}, 0, err
		}

		_, _, _ = cTxTime0, cTxTime1, cTxTime2
		_, _ = cRxTime0, cRxTime1

		break
	}

	return
}
