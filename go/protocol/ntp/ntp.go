package ntp

import (
	"encoding/binary"
	"errors"
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

type Timestamp32 struct {
	Seconds uint16
	Fraction uint16
}

type Timestamp64 struct {
	Seconds uint32
	Fraction uint32
}

type Packet struct {
	LVM            uint8
	Stratum	       uint8
	Poll           int8
	Precision      int8
	RootDelay      Timestamp32
	RootDispersion Timestamp32
	ReferenceID    uint32
	ReferenceTime  Timestamp64
	OriginTime     Timestamp64
	ReceiveTime    Timestamp64
	TransmitTime   Timestamp64
}

var errUnexpectedPacketSize = errors.New("unexpected packet size")

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
	return (lvm >> 6) & 0x3
}

func Version(lvm uint8) uint8 {
	return (lvm >> 3) & 0x7
}

func Mode(lvm uint8) uint8 {
	return lvm & 0x7
}
