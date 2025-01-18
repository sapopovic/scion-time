package csptp_test

// Based on an Claude AI interaction

import (
	"math"
	"testing"

	"example.com/scion-time/net/csptp"
)

func TestSdoIDMessageTypeRoundTrip(t *testing.T) {
	vs := []uint8{0, 1, math.MaxUint8 - 1, math.MaxUint8,
		csptp.MessageTypeSync, csptp.MessageTypeFollowUp}
	for _, v := range vs {
		msg0 := csptp.Message{SdoIDMessageType: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.SdoIDMessageType != v {
			t.Fail()
		}
		if msg1.SdoIDMessageType != msg0.SdoIDMessageType {
			t.Fail()
		}
	}
}

func TestPTPVersionRoundTrip(t *testing.T) {
	vs := []uint8{0, 1, math.MaxUint8 - 1, math.MaxUint8,
		csptp.PTPVersion}
	for _, v := range vs {
		msg0 := csptp.Message{PTPVersion: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.PTPVersion != v {
			t.Fail()
		}
		if msg1.PTPVersion != msg0.PTPVersion {
			t.Fail()
		}
	}
}

func TestMessageLengthRoundTrip(t *testing.T) {
	vs := []uint16{0, 1, math.MaxUint16 - 1, math.MaxUint16}
	for _, v := range vs {
		msg0 := csptp.Message{MessageLength: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.MessageLength != v {
			t.Fail()
		}
		if msg1.MessageLength != msg0.MessageLength {
			t.Fail()
		}
	}
}

func TestDomainNumberRoundTrip(t *testing.T) {
	vs := []uint8{0, 1, math.MaxUint8 - 1, math.MaxUint8}
	for _, v := range vs {
		msg0 := csptp.Message{DomainNumber: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.DomainNumber != v {
			t.Fail()
		}
		if msg1.DomainNumber != msg0.DomainNumber {
			t.Fail()
		}
	}
}

func TestMinorSdoIDRoundTrip(t *testing.T) {
	vs := []uint8{0, 1, math.MaxUint8 - 1, math.MaxUint8}
	for _, v := range vs {
		msg0 := csptp.Message{MinorSdoID: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.MinorSdoID != v {
			t.Fail()
		}
		if msg1.MinorSdoID != msg0.MinorSdoID {
			t.Fail()
		}
	}
}

func TestFlagFieldRoundTrip(t *testing.T) {
	vs := []uint16{0, 1, math.MaxUint16 - 1, math.MaxUint16,
		csptp.FlagTwoStep, csptp.FlagUnicast, csptp.FlagCurrentUTCOffsetValid, csptp.FlagPTPTimescale}
	for _, v := range vs {
		msg0 := csptp.Message{FlagField: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.FlagField != v {
			t.Fail()
		}
		if msg1.FlagField != msg0.FlagField {
			t.Fail()
		}
	}
}

func TestCorrectionFieldRoundTrip(t *testing.T) {
	vs := []int64{math.MinInt64, math.MinInt64 + 1, -1, 0, 1, math.MaxInt64 - 1, math.MaxInt64}
	for _, v := range vs {
		msg0 := csptp.Message{CorrectionField: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.CorrectionField != v {
			t.Fail()
		}
		if msg1.CorrectionField != msg0.CorrectionField {
			t.Fail()
		}
	}
}

func TestMessageTypeSpecificRoundTrip(t *testing.T) {
	vs := []uint32{0, 1, math.MaxUint32 - 1, math.MaxUint32}
	for _, v := range vs {
		msg0 := csptp.Message{MessageTypeSpecific: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.MessageTypeSpecific != v {
			t.Fail()
		}
		if msg1.MessageTypeSpecific != msg0.MessageTypeSpecific {
			t.Fail()
		}
	}
}

func TestSourcePortIdentityRoundTrip(t *testing.T) {
	vs := []csptp.PortID{
		{ClockID: 0, Port: 0},
		{ClockID: 0, Port: math.MaxUint16},
		{ClockID: 1, Port: 0},
		{ClockID: 1, Port: math.MaxUint16},
		{ClockID: math.MaxUint64 - 1, Port: 0},
		{ClockID: math.MaxUint64 - 1, Port: math.MaxUint16},
		{ClockID: math.MaxUint64, Port: 0},
		{ClockID: math.MaxUint64, Port: math.MaxUint16},
	}
	for _, v := range vs {
		msg0 := csptp.Message{SourcePortIdentity: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.SourcePortIdentity != v {
			t.Fail()
		}
		if msg1.SourcePortIdentity != msg0.SourcePortIdentity {
			t.Fail()
		}
	}
}

func TestSequenceIDRoundTrip(t *testing.T) {
	vs := []uint16{0, 1, math.MaxUint16 - 1, math.MaxUint16}
	for _, v := range vs {
		msg0 := csptp.Message{SequenceID: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.SequenceID != v {
			t.Fail()
		}
		if msg1.SequenceID != msg0.SequenceID {
			t.Fail()
		}
	}
}

func TestControlFieldRoundTrip(t *testing.T) {
	vs := []uint8{0, 1, math.MaxUint8 - 1, math.MaxUint8,
		csptp.ControlSync, csptp.ControlFollowUp, csptp.ControlOther}
	for _, v := range vs {
		msg0 := csptp.Message{ControlField: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.ControlField != v {
			t.Fail()
		}
		if msg1.ControlField != msg0.ControlField {
			t.Fail()
		}
	}
}

func TestLogMessageIntervalRoundTrip(t *testing.T) {
	vs := []int8{math.MinInt8, math.MinInt8 + 1, -1, 0, 1, math.MaxInt8 - 1, math.MaxInt8,
		csptp.LogMessageInterval}
	for _, v := range vs {
		msg0 := csptp.Message{LogMessageInterval: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.LogMessageInterval != v {
			t.Fail()
		}
		if msg1.LogMessageInterval != msg0.LogMessageInterval {
			t.Fail()
		}
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	vs := []csptp.Timestamp{
		{
			Seconds:     [6]uint8{0, 0, 0, 0, 0, 0},
			Nanoseconds: 0,
		},
		{
			Seconds:     [6]uint8{0, 0, 0, 0, 0, 1},
			Nanoseconds: 1,
		},
		{
			Seconds:     [6]uint8{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE},
			Nanoseconds: math.MaxUint32 - 1,
		},
		{
			Seconds:     [6]uint8{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			Nanoseconds: math.MaxUint32,
		},
	}
	for _, v := range vs {
		msg0 := csptp.Message{Timestamp: v}
		b := make([]byte, csptp.MinMessageLength)
		csptp.EncodeMessage(b, &msg0)
		var msg1 csptp.Message
		err := csptp.DecodeMessage(&msg1, b)
		if err != nil {
			t.Fatal(err)
		}
		if msg0.Timestamp != v {
			t.Fail()
		}
		if msg1.Timestamp != msg0.Timestamp {
			t.Fail()
		}
	}
}

func TestCompleteMessageRoundTrip(t *testing.T) {
	msg0 := csptp.Message{
		SdoIDMessageType:    csptp.MessageTypeSync,
		PTPVersion:          csptp.PTPVersion,
		MessageLength:       csptp.MinMessageLength,
		DomainNumber:        csptp.DomainNumber,
		MinorSdoID:          0,
		FlagField:           csptp.FlagTwoStep | csptp.FlagUnicast,
		CorrectionField:     0x123456789ABCDEF,
		MessageTypeSpecific: 0xDEADBEEF,
		SourcePortIdentity: csptp.PortID{
			ClockID: 0xAAAABBBBCCCCDDDD,
			Port:    0xEEEE,
		},
		SequenceID:         0xFFFF,
		ControlField:       csptp.ControlSync,
		LogMessageInterval: csptp.LogMessageInterval,
		Timestamp: csptp.Timestamp{
			Seconds:     [6]uint8{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
			Nanoseconds: 0x77777777,
		},
	}
	b := make([]byte, csptp.MinMessageLength)
	csptp.EncodeMessage(b, &msg0)
	var msg1 csptp.Message
	err := csptp.DecodeMessage(&msg1, b)
	if err != nil {
		t.Fatal(err)
	}
	if msg1 != msg0 {
		t.Fail()
	}
}
