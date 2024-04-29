package filters_test

import (
	"testing"
	"time"

	"example.com/scion-time/net/ntp"

	"example.com/scion-time/core/sync/flash/filters"
)

func TestFilter(t *testing.T) {
	f := filters.NewLuckyPacketFilter(3 /* cap */, 1 /* pick */)

	cTxTime := time.Time{}
	sRxTime := cTxTime.Add(9 * time.Millisecond)
	sTxTime := sRxTime.Add(1 * time.Millisecond)
	cRxTime := sTxTime.Add(11 * time.Millisecond)
	off0 := f.Do("test", cTxTime, sRxTime, sTxTime, cRxTime)
	off1 := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	if off0 != off1 {
		t.Errorf("filter(%v, %v, %v, %v) == %v; want %v",
			cTxTime, sRxTime, sTxTime, cRxTime, off0, off1)
	}
}
