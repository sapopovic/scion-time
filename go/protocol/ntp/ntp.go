package ntp

import (
	"encoding/binary"
	"errors"
	"time"
)

const (
	ServerPort  = 123
	ServerRefID = 0x58535453

	LeapIndicatorNoWarning    = 0
	LeapIndicatorInsertSecond = 1
	LeapIndicatorDeleteSecond = 2
  LeapIndicatorUnknown      = 3

  VersionMin = 1
  VersionMax = 4

  ModeReserved0        = 0
  ModeSymmetricActive  = 1
  ModeSymmetricPassive = 2
  ModeClient           = 3
  ModeServer           = 4
  ModeBroadcast        = 5
  ModeControl          = 6
  ModeReserved7        = 7
)

type Time32 struct {
	Seconds uint16
	Fraction uint16
}

type Time64 struct {
	Seconds uint32
	Fraction uint32
}

type Packet struct {
	LVM            uint8
	Stratum	       uint8
	Poll           int8
	Precision      int8
	RootDelay      Time32
	RootDispersion Time32
	ReferenceID    uint32
	ReferenceTime  Time64
	OriginTime     Time64
	ReceiveTime    Time64
	TransmitTime   Time64
}

var (
	epoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

	errUnexpectedPacketSize = errors.New("unexpected packet size")
)

func Time64FromTime(t time.Time) Time64 {
	d := t.Sub(epoch).Nanoseconds()
	return Time64{
		Seconds: uint32(
			d / time.Second.Nanoseconds()),
		Fraction: uint32(
			(d % time.Second.Nanoseconds() << 32 + time.Second.Nanoseconds() / 2) / time.Second.Nanoseconds()),
	}
}

func DecodePacket(b []byte, pkt *Packet) error {
	if len(b) != 48 {
		return errUnexpectedPacketSize
	}

	pkt.LVM = uint8(b[0])
	pkt.Stratum = uint8(b[1])
	pkt.Poll = int8(b[2])
	pkt.Precision = int8(b[3])
	pkt.RootDelay.Seconds = binary.BigEndian.Uint16(b[4:])
	pkt.RootDelay.Fraction = binary.BigEndian.Uint16(b[6:])
	pkt.RootDispersion.Seconds = binary.BigEndian.Uint16(b[8:])
	pkt.RootDispersion.Fraction = binary.BigEndian.Uint16(b[10:])
	pkt.ReferenceID = binary.BigEndian.Uint32(b[12:])
	pkt.ReferenceTime.Seconds = binary.BigEndian.Uint32(b[16:])
	pkt.ReferenceTime.Fraction = binary.BigEndian.Uint32(b[20:])
	pkt.OriginTime.Seconds = binary.BigEndian.Uint32(b[24:])
	pkt.OriginTime.Fraction = binary.BigEndian.Uint32(b[28:])
	pkt.ReceiveTime.Seconds = binary.BigEndian.Uint32(b[32:])
	pkt.ReceiveTime.Fraction = binary.BigEndian.Uint32(b[36:])
	pkt.TransmitTime.Seconds = binary.BigEndian.Uint32(b[40:])
	pkt.TransmitTime.Fraction = binary.BigEndian.Uint32(b[44:])
	return nil
}

func LeapIndicator(lvm uint8) uint8 {
	return (lvm >> 6) & 0b0000_0011
}

func SetLeapIndicator(lvm *uint8, l uint8) {
	if l & 0b0000_0011 != l {
		panic("unexpected NTP leap indicator value")
	}
	*lvm = (*lvm & 0b0011_1111) | (l << 6)
}

func Version(lvm uint8) uint8 {
	return (lvm >> 3) & 0b0000_0111
}

func SetVersion(lvm *uint8, v uint8) {
	if v & 0b0000_0111 != v {
		panic("unexpected NTP version value")
	}
	*lvm = (*lvm & 0b_1100_0111) | (v << 3)
}

func Mode(lvm uint8) uint8 {
	return lvm & 0b0000_0111
}

func SetMode(lvm *uint8, m uint8) {
	if m & 0b0000_0111 != m {
		panic("unexpected NTP mode value")
	}
	*lvm = (*lvm & 0b1111_1000) | m
}

