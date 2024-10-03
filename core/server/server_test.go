package server_test

import (
	"log/slog"
	"testing"
	"time"

	"example.com/scion-time/base/logbase"

	"example.com/scion-time/core/server"
	"example.com/scion-time/core/timebase"

	"example.com/scion-time/driver/clocks"

	"example.com/scion-time/net/ntp"
)

func init() {
	lclk := clocks.NewSystemClock(
		slog.New(logbase.NewNopHandler()),
		clocks.UnknownDrift,
	)
	timebase.RegisterClock(lclk)
}

func TestSimpleRequest(t *testing.T) {
	server.LogTSS(t, "pre")

	cTxTime := timebase.Now()
	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)

	rxt := timebase.Now()
	clientID := "client-0"

	var txt0 time.Time
	var ntpresp ntp.Packet
	server.HandleRequest(clientID, &ntpreq, &rxt, &txt0, &ntpresp)

	server.LogTSS(t, "post")
}
