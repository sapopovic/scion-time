// Driver for quick experiments

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"time"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/core/client"
	"example.com/scion-time/core/server"
	"example.com/scion-time/core/timebase"
	"example.com/scion-time/driver/clocks"
)

func runX() {
	var (
		laddr, raddr string
		dscp         uint
		periodic     bool
	)

	toolFlags := flag.NewFlagSet("tool", flag.ExitOnError)
	toolFlags.StringVar(&laddr, "local", "", "Local address")
	toolFlags.StringVar(&raddr, "remote", "", "Remote address")
	toolFlags.UintVar(&dscp, "dscp", 0, "Differentiated services codepoint, must be in range [0, 63]")
	toolFlags.BoolVar(&periodic, "periodic", false, "Perform periodic offset measurements")

	err := toolFlags.Parse(os.Args[2:])
	if err != nil || toolFlags.NArg() != 0 {
		panic("failed to parse arguments")
	}

	initLogger(true /* verbose */)
	log := slog.Default()

	ctx := context.Background()

	lclk := clocks.NewSystemClock(log, clocks.UnknownDrift)
	timebase.RegisterClock(lclk)

	if raddr == "" {
		localAddr, err := net.ResolveUDPAddr("udp", laddr)
		if err != nil {
			panic(err)
		}

		server.StartCSPTPServerIP(ctx, log, localAddr, 0)

		select{}
	} else {
		localAddr := netip.MustParseAddr(laddr)
		remoteAddr := netip.MustParseAddr(raddr)

		c := &client.CSPTPClientIP{
			Log:  log,
			DSCP: uint8(dscp),
		}

		for {
			ts, off, err := c.MeasureClockOffset(ctx, localAddr, remoteAddr)
			if err != nil {
				logbase.Fatal(slog.Default(), "failed to measure clock offset", slog.Any("remote", raddr), slog.Any("error", err))
			}
			if !periodic {
				break
			}
			fmt.Printf("%s,%+.9f\n", ts.UTC().Format(time.RFC3339), off.Seconds())
			lclk.Sleep(1 * time.Second)
		}
	}
}
