package csptp

// See FlashPTP at https://github.com/meinberg-sync/flashptpd
// and IEEE 1588-2019, PTP version 2.1

import (
	"time"
)

const (
	nanosecondsPerSecond int64 = 1e9

	EventPortIP      = 319   // Sync
	EventPortSCION   = 10319 // Sync
	GeneralPortIP    = 320   // Follow Up
	GeneralPortSCION = 10320 // Follow Up

	SdoID = 0

	MessageTypeSync     = 0
	MessageTypeFollowUp = 8

	VersionMin     = 1
	VersionMax     = 0x12
	VersionDefault = 0x12

	DomainNumber = 0
)

type PortID struct {
	ClockID uint64
	Port    uint16
}

type Timestamp struct {
	Seconds     [6]uint8
	Nanoseconds uint32
}

type Packet struct {
	SdoIDMessageType    uint8
	Version             uint8
	MessageLength       uint16
	DomainNumber        uint8
	MinorSdoID          uint8
	FlagField           uint16
	CorrectionField     int64
	MessageTypeSpecific uint32
	SourcePortIdentity  PortID
	SequenceID          uint16
	ControlField        uint8
	LogMessageInterval  int8
	Timestamp           Timestamp
}

func secondsFromPTPSeconds(s [6]uint8) uint64 {
	return uint64(s[0])<<40 | uint64(s[1])<<32 | uint64(s[2])<<24 |
		uint64(s[3])<<16 | uint64(s[4])<<8 | uint64(s[5])
}

func TimestampFromTime(t time.Time) Timestamp {
	unixSec := t.Unix()
	if unixSec < 0 {
		panic("invalid argument: t must not be before the epoch")
	}
	s := uint64(unixSec)
	return Timestamp{
		Seconds: [6]uint8{
			uint8(s >> 40), uint8(s >> 32), uint8(s >> 24),
			uint8(s >> 16), uint8(s >> 8), uint8(s)},
		Nanoseconds: uint32(t.Nanosecond()),
	}
}

func (x Timestamp) Sub(y Timestamp) time.Duration {
	return time.Duration(
		(int64(secondsFromPTPSeconds(x.Seconds))-int64(secondsFromPTPSeconds(y.Seconds)))*nanosecondsPerSecond +
			(int64(x.Nanoseconds) - int64(y.Nanoseconds)))
}

func DecodePacket(pkt *Packet, b []byte) error {
	return nil
}

func EncodePacket(b *[]byte, pkt *Packet) {}
