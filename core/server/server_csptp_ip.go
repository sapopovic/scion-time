package server

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"time"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/core/timebase"
	"example.com/scion-time/net/csptp"
	"example.com/scion-time/net/udp"
)

func runCSPTPServerIP(ctx context.Context, log *slog.Logger,
	conn *net.UDPConn, iface string, dscp uint8) {
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

		// decode packet

		// validate request

		// log request
		_ = rxt

		var txt0 time.Time
		// handle request: req -> resp

		// encode response

		n, err = conn.WriteToUDPAddrPort(buf, srcAddr)
		if err != nil || n != len(buf) {
			log.LogAttrs(ctx, slog.LevelError, "failed to write packet", slog.Any("error", err))
			continue
		}
		txt1, id, err := udp.ReadTXTimestamp(conn)
		if err != nil {
			txt1 = txt0
			log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp",
				slog.Any("error", err))
		} else if id != txID {
			txt1 = txt0
			log.LogAttrs(ctx, slog.LevelError, "failed to read packet tx timestamp",
				slog.Uint64("id", uint64(id)), slog.Uint64("expected", uint64(txID)))
			txID = id + 1
		} else {
			txID++
		}

		// update tx timestamp
		_ = txt1
	}
}

func StartCSPTPServerIP(ctx context.Context, log *slog.Logger,
	localHost *net.UDPAddr, dscp uint8) {
	log.LogAttrs(ctx, slog.LevelInfo, "CSPTP server listening via IP",
		slog.Any("local host", localHost),
	)

	lc := net.ListenConfig{
		Control: udp.SetsockoptReuseAddrPort,
	}
	address := net.JoinHostPort(localHost.IP.String(), strconv.Itoa(localHost.Port))
	for range ipServerNumGoroutine {
		conn, err := lc.ListenPacket(ctx, "udp", address)
		if err != nil {
			logbase.FatalContext(ctx, log, "failed to listen for packets", slog.Any("error", err))
		}
		go runCSPTPServerIP(ctx, log, conn.(*net.UDPConn), localHost.Zone, dscp)
	}
}
