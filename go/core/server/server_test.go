package server_test

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"example.com/scion-time/go/core/server"
	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/driver/clock"

	"example.com/scion-time/go/net/ntp"
)

func init() {
	lclk := &clock.SystemClock{Log: zap.NewNop()}
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
