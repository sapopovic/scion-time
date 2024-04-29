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

var t0 time.Time

func offset(m measurement) time.Duration {
	return ntp.ClockOffset(m.cTxTime, m.sRxTime, m.sTxTime, m.cRxTime)
}

func doFilter(f *filters.LuckyPacketFilter, m measurement) time.Duration {
	return f.Do("test", m.cTxTime, m.sRxTime, m.sTxTime, m.cRxTime)
}

func TestFilter0(t *testing.T) {
	f := filters.NewLuckyPacketFilter(3 /* cap */, 1 /* pick */)
	x := measurement{
		cTxTime: t0,
		sRxTime: t0.Add(10 * time.Millisecond),
		sTxTime: t0.Add(11 * time.Millisecond),
		cRxTime: t0.Add(21 * time.Millisecond),
	}
	off0 := doFilter(f, x)
	off1 := offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter1(t *testing.T) {
	f := filters.NewLuckyPacketFilter(3 /* cap */, 1 /* pick */)
	x := measurement{
		cTxTime: t0,
		sRxTime: t0.Add(9 * time.Millisecond),
		sTxTime: t0.Add(10 * time.Millisecond),
		cRxTime: t0.Add(21 * time.Millisecond),
	}
	off0 := doFilter(f, x)
	off1 := offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter2(t *testing.T) {
	f := filters.NewLuckyPacketFilter(3 /* cap */, 1 /* pick */)
	x := measurement{
		cTxTime: t0,
		sRxTime: t0.Add(11 * time.Millisecond),
		sTxTime: t0.Add(12 * time.Millisecond),
		cRxTime: t0.Add(21 * time.Millisecond),
	}
	off0 := doFilter(f, x)
	off1 := offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter3(t *testing.T) {
	f := filters.NewLuckyPacketFilter(3 /* cap */, 1 /* pick */)
	x := measurement{
		cTxTime: t0,
		sRxTime: t0.Add(10 * time.Millisecond),
		sTxTime: t0.Add(11 * time.Millisecond),
		cRxTime: t0.Add(21 * time.Millisecond),
	}
	y := measurement{
		cTxTime: t0,
		sRxTime: t0.Add(19 * time.Millisecond),
		sTxTime: t0.Add(20 * time.Millisecond),
		cRxTime: t0.Add(41 * time.Millisecond),
	}
	var off0, off1 time.Duration
	off0 = doFilter(f, x)
	off1 = offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0 = doFilter(f, y)
	off1 = offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0 = doFilter(f, y)
	off1 = offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0 = doFilter(f, y)
	off1 = offset(y)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

