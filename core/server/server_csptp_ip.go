package server

import (
	"context"
	"log/slog"
	"net"
	"strconv"

	"example.com/scion-time/base/logbase"

	"example.com/scion-time/net/udp"
)

func runCSPTPServerIP(ctx context.Context, log *slog.Logger,
	conn *net.UDPConn, iface string, dscp uint8) {
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
