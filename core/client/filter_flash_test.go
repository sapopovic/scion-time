package client_test

import (
	"testing"
	"time"

	"example.com/scion-time/net/ntp"

	"example.com/scion-time/core/client"
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

func filter(f *client.LuckyPacketFilter, m measurement) time.Duration {
	return f.Do(m.cTxTime, m.sRxTime, m.sTxTime, m.cRxTime)
}

func TestFilter0(t *testing.T) {
	f := &client.LuckyPacketFilter{}
	x := measurement{cTxTime: at(0), sRxTime: at(10), sTxTime: at(10), cRxTime: at(20)}
	off0, off1 := filter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter1(t *testing.T) {
	f := &client.LuckyPacketFilter{}
	x := measurement{cTxTime: at(0), sRxTime: at(9), sTxTime: at(9), cRxTime: at(20)}
	off0, off1 := filter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter2(t *testing.T) {
	f := &client.LuckyPacketFilter{}
	x := measurement{cTxTime: at(0), sRxTime: at(11), sTxTime: at(11), cRxTime: at(20)}
	off0, off1 := filter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter3(t *testing.T) {
	f := client.NewLuckyPacketFilter(3 /* cap */, 1 /* pick */)
	a := measurement{cTxTime: at(0), sRxTime: at(19), sTxTime: at(19), cRxTime: at(40)}
	x := measurement{cTxTime: at(0), sRxTime: at(10), sTxTime: at(10), cRxTime: at(20)}
	var off0, off1 time.Duration
	off0, off1 = filter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0, off1 = filter(f, a), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0, off1 = filter(f, a), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
	off0, off1 = filter(f, a), offset(a)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter4(t *testing.T) {
	f := client.NewLuckyPacketFilter(5 /* cap */, 1 /* pick */)
	a := measurement{cTxTime: at(0), sRxTime: at(19), sTxTime: at(19), cRxTime: at(40)}
	x := measurement{cTxTime: at(0), sRxTime: at(10), sTxTime: at(10), cRxTime: at(20)}
	_ = filter(f, a)
	_ = filter(f, a)
	_ = filter(f, x)
	_ = filter(f, a)
	_ = filter(f, a)
	var off0, off1 time.Duration
	off0, off1 = filter(f, a), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter5(t *testing.T) {
	f := client.NewLuckyPacketFilter(5 /* cap */, 5 /* pick */)
	a := measurement{cTxTime: at(0), sRxTime: at(19), sTxTime: at(19), cRxTime: at(40)}
	b := measurement{cTxTime: at(0), sRxTime: at(21), sTxTime: at(21), cRxTime: at(40)}
	x := measurement{cTxTime: at(0), sRxTime: at(10), sTxTime: at(10), cRxTime: at(20)}
	_ = filter(f, a)
	_ = filter(f, a)
	_ = filter(f, b)
	_ = filter(f, b)
	var off0, off1 time.Duration
	off0, off1 = filter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}

func TestFilter6(t *testing.T) {
	f := client.NewLuckyPacketFilter(5 /* cap */, 3 /* pick */)
	a := measurement{cTxTime: at(0), sRxTime: at(29), sTxTime: at(29), cRxTime: at(60)}
	b := measurement{cTxTime: at(0), sRxTime: at(19), sTxTime: at(19), cRxTime: at(40)}
	c := measurement{cTxTime: at(0), sRxTime: at(21), sTxTime: at(21), cRxTime: at(40)}
	d := measurement{cTxTime: at(0), sRxTime: at(31), sTxTime: at(31), cRxTime: at(60)}
	x := measurement{cTxTime: at(0), sRxTime: at(10), sTxTime: at(10), cRxTime: at(20)}
	_ = filter(f, a)
	_ = filter(f, b)
	_ = filter(f, c)
	_ = filter(f, d)
	var off0, off1 time.Duration
	off0, off1 = filter(f, x), offset(x)
	if off0 != off1 {
		t.Errorf("got %q, want %q", off0, off1)
	}
}
