package ntp

import (
	"encoding/binary"
	"errors"
)

const ServerRefID = 0x58535453

type Timestamp32 struct {
	Seconds uint16
	Fraction uint16
}

type Timestamp64 struct {
	Seconds uint32
	Fraction uint32
}

type Packet struct {
	LIVNMode       uint8
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

var errInvalidPacketSize = errors.New("invalid packet size")

func DecodePacket(b []byte, pkt *Packet) error {
	if len(b) != 48 {
		return errInvalidPacketSize
	}

	pkt.LIVNMode = uint8(b[0])
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
