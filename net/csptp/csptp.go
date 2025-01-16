package csptp

// See FlashPTP at https://github.com/meinberg-sync/flashptpd
// and IEEE 1588-2019, PTP version 2.1

import (
	"time"
)

const (
	EventPortIP      = 319   // Sync
	EventPortSCION   = 10319 // Sync
	GeneralPortIP    = 320   // Follow Up
	GeneralPortSCION = 10320 // Follow Up

	SdoID = 0

	MessageTypeSync     = 0
	MessageTypeFollowUp = 8

	DefaultVersion = 0x12

	DomainNumber = 0

	FlagTwoStep               = 1 << 9
	FlagUnicast               = 1 << 10
	FlagCurrentUTCOffsetValid = 1 << 2
	FlagPTPTimescale          = 1 << 3

	MessageControlSync     = 0
	MessageControlFollowUp = 2
	MessageControlOther    = 5

	LogMessageInterval = 0x7f

	TLVTypeOrganizationExtension = 3

	OrganizationIDMeinberg0 = 0xec
	OrganizationIDMeinberg1 = 0x46
	OrganizationIDMeinberg2 = 0x70

	OrganizationSubTypeRequest0 = 0x52
	OrganizationSubTypeRequest1 = 0x65
	OrganizationSubTypeRequest2 = 0x71

	OrganizationSubTypeResponse0 = 0x52
	OrganizationSubTypeResponse1 = 0x65
	OrganizationSubTypeResponse2 = 0x73

	TLVFlagServerStateDS = 1 << 0

	ErrorTxTimestampInvalid = 1
)

type PortID struct {
	ClockID uint64
	Port    uint16
}

type Timestamp struct {
	Seconds     [6]uint8
	Nanoseconds uint32
}

type Message struct {
	SdoIDType          uint8
	Version            uint8
	Length             uint16
	DomainNumber       uint8
	MinorSdoID         uint8
	Flags              uint16
	CorrectionField    int64
	TypeSpecific       uint32
	SourcePortIdentity PortID
	SequenceID         uint16
	ControlField       uint8
	LogMessageInterval int8
	Timestamp          Timestamp
}

type RequestTLV struct {
	Type                uint16
	Length              uint16
	OrganizationID      [3]uint8
	OrganizationSubType [3]uint8
	Flags               uint32
}

type ServerStateDS struct {
	GMPriority1     uint8
	GMClockClass    uint8
	GMClockAccuracy uint8
	GMClockVariance uint16
	GMPriority2     uint8
	GMClockID       uint64
	StepsRemoved    uint16
	TimeSource      uint8
	Reserved        uint8
}

type ResponseTLV struct {
	Type                    uint16
	Length                  uint16
	OrganizationID          [3]uint8
	OrganizationSubType     [3]uint8
	Flags                   uint32
	Error                   uint16
	RequestIngressTimestamp Timestamp
	RequestCorrectionField  int64
	UTCOffset               int16
	ServerStateDS           ServerStateDS
}

//lint:ignore U1000 work in progress
func flagField(twoStep bool) uint16 {
	f := uint16(FlagUnicast)
	if twoStep {
		f |= FlagTwoStep
	}
	return f
}

func TimestampFromTime(t time.Time) Timestamp {
	s := t.Unix()
	if s < 0 {
		panic("invalid argument: t must not be before 1970-01-01T00:00:00Z")
	}
	if s > 1<<48-1 {
		panic("invalid argument: t must not be after 8921556-12-07T10:44:15.999999999Z")
	}
	return Timestamp{
		Seconds: [6]uint8{
			uint8(uint64(s) >> 40), uint8(uint64(s) >> 32), uint8(uint64(s) >> 24),
			uint8(uint64(s) >> 16), uint8(uint64(s) >> 8), uint8(uint64(s))},
		Nanoseconds: uint32(t.Nanosecond()),
	}
}

func TimeFromTimestamp(t Timestamp) time.Time {
	s := uint64(t.Seconds[0])<<40 | uint64(t.Seconds[1])<<32 | uint64(t.Seconds[2])<<24 |
		uint64(t.Seconds[3])<<16 | uint64(t.Seconds[4])<<8 | uint64(t.Seconds[5])
	return time.Unix(int64(s), int64(t.Nanoseconds)).UTC()
}

func DurationFromTimeInterval(i int64) time.Duration {
	return time.Duration(i >> 16)
}

func DecodeMessage(msg *Message, b []byte) error {
	return nil
}

func EncodeMessage(b *[]byte, msg *Message) {}
