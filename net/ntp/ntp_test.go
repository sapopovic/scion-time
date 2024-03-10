package ntp_test

import (
	"math"
	"testing"
	"time"

	"example.com/scion-time/net/ntp"
)

func TestTime64Conversion(t *testing.T) {
	t0 := time.Now()
	t64 := ntp.Time64FromTime(t0)
	t1 := ntp.TimeFromTime64(t64)

	if !t1.Equal(t0) {
		t.Errorf("t1 and t0 must be equal")
	}
}

func TestBeforeAfter(t *testing.T) {
	t0 := ntp.Time64{Seconds: 10, Fraction: 0}
	t1 := ntp.Time64{Seconds: 20, Fraction: 0}

	if !t0.Before(t1) {
		t.Errorf("t0 must be before t1")
	}
	if t1.Before(t0) {
		t.Errorf("t1 must not be before t0")
	}
	if !t1.After(t0) {
		t.Errorf("t1 must be after t0")
	}
	if t0.After(t1) {
		t.Errorf("t0 must not be after t1")
	}
}

func TestBeforeAfterWithFraction(t *testing.T) {
	t0 := ntp.Time64{Seconds: 10, Fraction: 0}
	t1 := ntp.Time64{Seconds: 20, Fraction: 0}
	t2 := ntp.Time64{Seconds: 10, Fraction: 100}
	t3 := ntp.Time64{Seconds: 10, Fraction: 200}

	// Testing with Fraction = 0
	if !t0.Before(t1) {
		t.Errorf("t0 must be before t1")
	}
	if t1.Before(t0) {
		t.Errorf("t1 must not be before t0")
	}
	if !t1.After(t0) {
		t.Errorf("t1 must be after t0")
	}
	if t0.After(t1) {
		t.Errorf("t0 must not be after t1")
	}

	// Testing with non-zero Fraction
	if !t2.Before(t3) {
		t.Errorf("t2 must be before t3")
	}
	if t3.Before(t2) {
		t.Errorf("t3 must not be before t2")
	}
	if !t3.After(t2) {
		t.Errorf("t3 must be after t2")
	}
	if t2.After(t3) {
		t.Errorf("t2 must not be after t3")
	}
}

func TestBeforeAfterVariousCases(t *testing.T) {
	// Case 1: Both Seconds and Fraction are zero
	t1 := ntp.Time64{Seconds: 0, Fraction: 0}
	t2 := ntp.Time64{Seconds: 0, Fraction: 0}

	if t1.Before(t2) {
		t.Errorf("t1 should not be before t2 when both are zero")
	}
	if t1.After(t2) {
		t.Errorf("t1 should not be after t2 when both are zero")
	}

	// Case 2: Non-zero Seconds, zero Fraction
	t3 := ntp.Time64{Seconds: 1, Fraction: 0}
	t4 := ntp.Time64{Seconds: 2, Fraction: 0}

	if !t3.Before(t4) {
		t.Errorf("t3 must be before t4 with non-zero seconds")
	}
	if !t4.After(t3) {
		t.Errorf("t4 must be after t3 with non-zero seconds")
	}

	// Case 3: Zero Seconds, non-zero Fraction
	t5 := ntp.Time64{Seconds: 0, Fraction: 100}
	t6 := ntp.Time64{Seconds: 0, Fraction: 200}

	if !t5.Before(t6) {
		t.Errorf("t5 must be before t6 with non-zero fraction")
	}
	if !t6.After(t5) {
		t.Errorf("t6 must be after t5 with non-zero fraction")
	}

	// Case 4: Both Seconds and Fraction are non-zero
	t7 := ntp.Time64{Seconds: 1, Fraction: 100}
	t8 := ntp.Time64{Seconds: 1, Fraction: 200}

	if !t7.Before(t8) {
		t.Errorf("t7 must be before t8 with both fields non-zero")
	}
	if !t8.After(t7) {
		t.Errorf("t8 must be after t7 with both fields non-zero")
	}
}

func TestLeapIndicatorRoundTrip(t *testing.T) {
	// Based on equivalent test from ntpd-rs
	for l := range uint8(4) {
		p0 := ntp.Packet{}
		p0.SetLeapIndicator(l)
		l0 := p0.LeapIndicator()
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		l1 := p1.LeapIndicator()
		if l0 != l {
			t.Fail()
		}
		if l1 != l0 {
			t.Fail()
		}
	}
}

func TestVersionRoundTrip(t *testing.T) {
	for v := range uint8(8) {
		p0 := ntp.Packet{}
		p0.SetVersion(v)
		v0 := p0.Version()
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		v1 := p1.Version()
		if v0 != v {
			t.Fail()
		}
		if v1 != v0 {
			t.Fail()
		}
	}
}

func TestModeRoundTrip(t *testing.T) {
	// Based on equivalent test from ntpd-rs
	for m := range uint8(8) {
		p0 := ntp.Packet{}
		p0.SetMode(m)
		m0 := p0.Mode()
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		m1 := p1.Mode()
		if m0 != m {
			t.Fail()
		}
		if m1 != m0 {
			t.Fail()
		}
	}
}

func TestStratumRoundTrip(t *testing.T) {
	vs := []uint8{0, 1, math.MaxUint8 - 1, math.MaxUint8}
	for _, v := range vs {
		p0 := ntp.Packet{Stratum: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.Stratum != v {
			t.Fail()
		}
		if p1.Stratum != p0.Stratum {
			t.Fail()
		}
	}
}

func TestPollRoundTrip(t *testing.T) {
	vs := []int8{math.MinInt8, math.MinInt8 + 1, -1, 0, 1, math.MaxInt8 - 1, math.MaxInt8}
	for _, v := range vs {
		p0 := ntp.Packet{Poll: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.Poll != v {
			t.Fail()
		}
		if p1.Poll != p0.Poll {
			t.Fail()
		}
	}
}

func TestPrecisionRoundTrip(t *testing.T) {
	vs := []int8{math.MinInt8, math.MinInt8 + 1, -1, 0, 1, math.MaxInt8 - 1, math.MaxInt8}
	for _, v := range vs {
		p0 := ntp.Packet{Precision: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.Precision != v {
			t.Fail()
		}
		if p1.Precision != p0.Precision {
			t.Fail()
		}
	}
}

func TestRootDelayRoundTrip(t *testing.T) {
	vs := []ntp.Time32{
		{Seconds: 0, Fraction: 0},
		{Seconds: 0, Fraction: 1},
		{Seconds: 0, Fraction: math.MaxUint16 - 1},
		{Seconds: 0, Fraction: math.MaxUint16},
		{Seconds: 1, Fraction: 0},
		{Seconds: 1, Fraction: 1},
		{Seconds: 1, Fraction: math.MaxUint16 - 1},
		{Seconds: 1, Fraction: math.MaxUint16},
		{Seconds: math.MaxUint16 - 1, Fraction: 0},
		{Seconds: math.MaxUint16 - 1, Fraction: 1},
		{Seconds: math.MaxUint16 - 1, Fraction: math.MaxUint16 - 1},
		{Seconds: math.MaxUint16 - 1, Fraction: math.MaxUint16},
		{Seconds: math.MaxUint16, Fraction: 0},
		{Seconds: math.MaxUint16, Fraction: 1},
		{Seconds: math.MaxUint16, Fraction: math.MaxUint16 - 1},
		{Seconds: math.MaxUint16, Fraction: math.MaxUint16},
	}
	for _, v := range vs {
		p0 := ntp.Packet{RootDelay: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.RootDelay.Seconds != v.Seconds {
			t.Fail()
		}
		if p0.RootDelay.Fraction != v.Fraction {
			t.Fail()
		}
		if p1.RootDelay.Seconds != p0.RootDelay.Seconds {
			t.Fail()
		}
		if p1.RootDelay.Fraction != p0.RootDelay.Fraction {
			t.Fail()
		}
	}
}

func TestRootDispersionRoundTrip(t *testing.T) {
	vs := []ntp.Time32{
		{Seconds: 0, Fraction: 0},
		{Seconds: 0, Fraction: 1},
		{Seconds: 0, Fraction: math.MaxUint16 - 1},
		{Seconds: 0, Fraction: math.MaxUint16},
		{Seconds: 1, Fraction: 0},
		{Seconds: 1, Fraction: 1},
		{Seconds: 1, Fraction: math.MaxUint16 - 1},
		{Seconds: 1, Fraction: math.MaxUint16},
		{Seconds: math.MaxUint16 - 1, Fraction: 0},
		{Seconds: math.MaxUint16 - 1, Fraction: 1},
		{Seconds: math.MaxUint16 - 1, Fraction: math.MaxUint16 - 1},
		{Seconds: math.MaxUint16 - 1, Fraction: math.MaxUint16},
		{Seconds: math.MaxUint16, Fraction: 0},
		{Seconds: math.MaxUint16, Fraction: 1},
		{Seconds: math.MaxUint16, Fraction: math.MaxUint16 - 1},
		{Seconds: math.MaxUint16, Fraction: math.MaxUint16},
	}
	for _, v := range vs {
		p0 := ntp.Packet{RootDispersion: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.RootDispersion.Seconds != v.Seconds {
			t.Fail()
		}
		if p0.RootDispersion.Fraction != v.Fraction {
			t.Fail()
		}
		if p1.RootDispersion.Seconds != p0.RootDispersion.Seconds {
			t.Fail()
		}
		if p1.RootDispersion.Fraction != p0.RootDispersion.Fraction {
			t.Fail()
		}
	}
}

func TestReferenceIDRoundTrip(t *testing.T) {
	vs := []uint32{0, 1, math.MaxUint32 - 1, math.MaxUint32}
	for _, v := range vs {
		p0 := ntp.Packet{ReferenceID: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.ReferenceID != v {
			t.Fail()
		}
		if p1.ReferenceID != p0.ReferenceID {
			t.Fail()
		}
	}
}

func TestReferenceTimeRoundTrip(t *testing.T) {
	vs := []ntp.Time64{
		{Seconds: 0, Fraction: 0},
		{Seconds: 0, Fraction: 1},
		{Seconds: 0, Fraction: math.MaxUint32 - 1},
		{Seconds: 0, Fraction: math.MaxUint32},
		{Seconds: 1, Fraction: 0},
		{Seconds: 1, Fraction: 1},
		{Seconds: 1, Fraction: math.MaxUint32 - 1},
		{Seconds: 1, Fraction: math.MaxUint32},
		{Seconds: math.MaxUint32 - 1, Fraction: 0},
		{Seconds: math.MaxUint32 - 1, Fraction: 1},
		{Seconds: math.MaxUint32 - 1, Fraction: math.MaxUint32 - 1},
		{Seconds: math.MaxUint32 - 1, Fraction: math.MaxUint32},
		{Seconds: math.MaxUint32, Fraction: 0},
		{Seconds: math.MaxUint32, Fraction: 1},
		{Seconds: math.MaxUint32, Fraction: math.MaxUint32 - 1},
		{Seconds: math.MaxUint32, Fraction: math.MaxUint32},
	}
	for _, v := range vs {
		p0 := ntp.Packet{ReferenceTime: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.ReferenceTime.Seconds != v.Seconds {
			t.Fail()
		}
		if p0.ReferenceTime.Fraction != v.Fraction {
			t.Fail()
		}
		if p1.ReferenceTime.Seconds != p0.ReferenceTime.Seconds {
			t.Fail()
		}
		if p1.ReferenceTime.Fraction != p0.ReferenceTime.Fraction {
			t.Fail()
		}
	}
}

func TestOriginTimeRoundTrip(t *testing.T) {
	vs := []ntp.Time64{
		{Seconds: 0, Fraction: 0},
		{Seconds: 0, Fraction: 1},
		{Seconds: 0, Fraction: math.MaxUint32 - 1},
		{Seconds: 0, Fraction: math.MaxUint32},
		{Seconds: 1, Fraction: 0},
		{Seconds: 1, Fraction: 1},
		{Seconds: 1, Fraction: math.MaxUint32 - 1},
		{Seconds: 1, Fraction: math.MaxUint32},
		{Seconds: math.MaxUint32 - 1, Fraction: 0},
		{Seconds: math.MaxUint32 - 1, Fraction: 1},
		{Seconds: math.MaxUint32 - 1, Fraction: math.MaxUint32 - 1},
		{Seconds: math.MaxUint32 - 1, Fraction: math.MaxUint32},
		{Seconds: math.MaxUint32, Fraction: 0},
		{Seconds: math.MaxUint32, Fraction: 1},
		{Seconds: math.MaxUint32, Fraction: math.MaxUint32 - 1},
		{Seconds: math.MaxUint32, Fraction: math.MaxUint32},
	}
	for _, v := range vs {
		p0 := ntp.Packet{OriginTime: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.OriginTime.Seconds != v.Seconds {
			t.Fail()
		}
		if p0.OriginTime.Fraction != v.Fraction {
			t.Fail()
		}
		if p1.OriginTime.Seconds != p0.OriginTime.Seconds {
			t.Fail()
		}
		if p1.OriginTime.Fraction != p0.OriginTime.Fraction {
			t.Fail()
		}
	}
}

func TestReceiveTimeRoundTrip(t *testing.T) {
	vs := []ntp.Time64{
		{Seconds: 0, Fraction: 0},
		{Seconds: 0, Fraction: 1},
		{Seconds: 0, Fraction: math.MaxUint32 - 1},
		{Seconds: 0, Fraction: math.MaxUint32},
		{Seconds: 1, Fraction: 0},
		{Seconds: 1, Fraction: 1},
		{Seconds: 1, Fraction: math.MaxUint32 - 1},
		{Seconds: 1, Fraction: math.MaxUint32},
		{Seconds: math.MaxUint32 - 1, Fraction: 0},
		{Seconds: math.MaxUint32 - 1, Fraction: 1},
		{Seconds: math.MaxUint32 - 1, Fraction: math.MaxUint32 - 1},
		{Seconds: math.MaxUint32 - 1, Fraction: math.MaxUint32},
		{Seconds: math.MaxUint32, Fraction: 0},
		{Seconds: math.MaxUint32, Fraction: 1},
		{Seconds: math.MaxUint32, Fraction: math.MaxUint32 - 1},
		{Seconds: math.MaxUint32, Fraction: math.MaxUint32},
	}
	for _, v := range vs {
		p0 := ntp.Packet{ReceiveTime: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.ReceiveTime.Seconds != v.Seconds {
			t.Fail()
		}
		if p0.ReceiveTime.Fraction != v.Fraction {
			t.Fail()
		}
		if p1.ReceiveTime.Seconds != p0.ReceiveTime.Seconds {
			t.Fail()
		}
		if p1.ReceiveTime.Fraction != p0.ReceiveTime.Fraction {
			t.Fail()
		}
	}
}

func TestTransmitTimeRoundTrip(t *testing.T) {
	vs := []ntp.Time64{
		{Seconds: 0, Fraction: 0},
		{Seconds: 0, Fraction: 1},
		{Seconds: 0, Fraction: math.MaxUint32 - 1},
		{Seconds: 0, Fraction: math.MaxUint32},
		{Seconds: 1, Fraction: 0},
		{Seconds: 1, Fraction: 1},
		{Seconds: 1, Fraction: math.MaxUint32 - 1},
		{Seconds: 1, Fraction: math.MaxUint32},
		{Seconds: math.MaxUint32 - 1, Fraction: 0},
		{Seconds: math.MaxUint32 - 1, Fraction: 1},
		{Seconds: math.MaxUint32 - 1, Fraction: math.MaxUint32 - 1},
		{Seconds: math.MaxUint32 - 1, Fraction: math.MaxUint32},
		{Seconds: math.MaxUint32, Fraction: 0},
		{Seconds: math.MaxUint32, Fraction: 1},
		{Seconds: math.MaxUint32, Fraction: math.MaxUint32 - 1},
		{Seconds: math.MaxUint32, Fraction: math.MaxUint32},
	}
	for _, v := range vs {
		p0 := ntp.Packet{TransmitTime: v}
		b := make([]byte, ntp.PacketLen)
		ntp.EncodePacket(&b, &p0)
		p1 := ntp.Packet{}
		err := ntp.DecodePacket(&p1, b)
		if err != nil {
			panic(err)
		}
		if p0.TransmitTime.Seconds != v.Seconds {
			t.Fail()
		}
		if p0.TransmitTime.Fraction != v.Fraction {
			t.Fail()
		}
		if p1.TransmitTime.Seconds != p0.TransmitTime.Seconds {
			t.Fail()
		}
		if p1.TransmitTime.Fraction != p0.TransmitTime.Fraction {
			t.Fail()
		}
	}
}
