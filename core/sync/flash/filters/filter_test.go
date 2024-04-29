package filters_test

import (
	"testing"
	"time"

	"example.com/scion-time/net/ntp"

	"example.com/scion-time/core/sync/flash/filters"
)

type measurement struct {
	cTxTime, sRxTime, sTxTime, cRxTime time.Time
}

func at(d int64) time.Time {
	var t0 time.Time
	return t0.Add(time.Duration(d) * time.Millisecond)
}

func offset(m measurement) time.Duration {
	return ntp.ClockOffset(m.cTxTime, m.sRxTime, m.sTxTime, m.cRxTime)
}

func doFilter(f *filters.LuckyPacketFilter, m measurement) time.Duration {
	return f.Do("test", m.cTxTime, m.sRxTime, m.sTxTime, m.cRxTime)
}

func TestFilter0(t *testing.T) {
	f := &filters.LuckyPacketFilter{}
	x := measurement{cTxTime: at(0), sRxTime: at(10), sTxTime: at(10), cRxTime: at(20)}
	off0, off1 := doFilter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter1(t *testing.T) {
	f := &filters.LuckyPacketFilter{}
	x := measurement{cTxTime: at(0), sRxTime: at(9), sTxTime: at(9), cRxTime: at(20)}
	off0, off1 := doFilter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter2(t *testing.T) {
	f := &filters.LuckyPacketFilter{}
	x := measurement{cTxTime: at(0), sRxTime: at(11), sTxTime: at(11), cRxTime: at(20)}
	off0, off1 := doFilter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter3(t *testing.T) {
	f := filters.NewLuckyPacketFilter(3 /* cap */, 1 /* pick */)
	x := measurement{cTxTime: at(0), sRxTime: at(10), sTxTime: at(10), cRxTime: at(20)}
	y := measurement{cTxTime: at(0), sRxTime: at(19), sTxTime: at(19), cRxTime: at(40)}
	var off0, off1 time.Duration
	off0, off1 = doFilter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0, off1 = doFilter(f, y), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0, off1 = doFilter(f, y), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0, off1 = doFilter(f, y), offset(y)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

