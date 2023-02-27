package core

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/net/ntp"
)

func init() {
	lclk := &SystemClock{Log: zap.NewNop()}
	timebase.RegisterClock(lclk)
}

func logTSS(t *testing.T, prefix string) {
	t.Helper()
	t.Logf("%s:tss = %+v", prefix, tss)
	t.Logf("%s:tssQ = %+v", prefix, tssQ)
}

func TestSimpleRequest(t *testing.T) {
	logTSS(t, "pre")

	cTxTime := timebase.Now()
	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)

	rxt := timebase.Now()
	clientID := "client-0"

	var txt0 time.Time
	var ntpresp ntp.Packet
	handleRequest(clientID, &ntpreq, &rxt, &txt0, &ntpresp)

	logTSS(t, "post")
}
