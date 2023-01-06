package ntp_test

import (
	"testing"
	"time"

	"example.com/scion-time/go/core"
	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/net/ntp"
)

func init() {
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
}

func TestSimpleRequest(t *testing.T) {
	ntp.LogTSS(t, "pre")

	cTxTime := timebase.Now()
	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)

	rxt := timebase.Now()
	clientID := "client-0"

	var txt0 time.Time
	var ntpresp ntp.Packet
	ntp.HandleRequest(clientID, &ntpreq, &rxt, &txt0, &ntpresp)

	ntp.LogTSS(t, "post")
}
