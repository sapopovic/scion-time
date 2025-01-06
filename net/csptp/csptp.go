package csptp

import (
	"time"
)

const (
	EventPortIP      = 319   // Sync
	EventPortSCION   = 10319 // Sync
	GeneralPortIP    = 320   // Follow Up
	GeneralPortSCION = 10320 // Follow Up

	SdoID = 0

	MsgTypeSync     = 0
	MsgTypeFollowUp = 8

	VersionMin     = 1
	VersionMax     = 0x12
	VersionDefault = 0x12

	Domain = 0
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
	SdoIDMsgType    uint8
	Version         uint8
	MsgLen          uint16
	Domain          uint8
	Flags           uint16
	Correction      int64
	MsgTypeSpecific uint32
	PortID          PortID
	SeqID           uint16
	MsgCtrl         uint8
	LogMsgPeriod    int8
	Timestamp       Timestamp
}

func secondsFromPTPSeconds(s [6]uint8) uint64 {
	return uint64(s[0])<<40 | uint64(s[1])<<32 | uint64(s[2])<<24 |
		uint64(s[3])<<16 | uint64(s[4])<<8 | uint64(s[5])
}

func TimestampFromTime(t time.Time) Timestamp {
	panic("not yet implemented")
	return Timestamp{}
}

func TimeFromTimestamp(t Timestamp) time.Time {
	return time.Unix(int64(secondsFromPTPSeconds(t.Seconds)), int64(t.Nanoseconds)).UTC()
}

func DecodePacket(pkt *Packet, b []byte) error {
	return nil
}

func EncodePacket(b *[]byte, pkt *Packet) {}
