package ntp

import (
	"encoding/binary"
	"errors"
	"time"
)

const (
	nanosecondsPerSecond int64 = 1e9

	ServerPortIP    = 123
	ServerPortSCION = 10123

	PacketLen = 48

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
	Seconds  uint16
	Fraction uint16
}

type Time64 struct {
	Seconds  uint32
	Fraction uint32
}

type Packet struct {
	LVM            uint8
	Stratum        uint8
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
	if d < 0 {
		panic("invalid argument: t must not be before the epoch")
	}
	return Time64{
		Seconds: uint32(
			d / nanosecondsPerSecond),
		Fraction: uint32(
			(d%nanosecondsPerSecond<<32 + nanosecondsPerSecond/2) / nanosecondsPerSecond),
	}
}

// TimeFromTime64 converts an NTP timestamp to a time.Time using a reference time t0
// to resolve the NTP timestamp era ambiguity.
func TimeFromTime64(t Time64, t0 time.Time) time.Time {
	const (
		// Seconds from Unix epoch (1970) to NTP epoch (1900), including leap days
		epoch = -(70*365 + 17) * 86400
		// Seconds per NTP era (2^32)
		secondsPerEra = 1 << 32
	)

	tref := t0.Unix()

	sec := epoch + (tref-epoch)/secondsPerEra*secondsPerEra + int64(t.Seconds)

	// If the timestamp would be too far in the past relative to
	// the reference time, assume it's from the next era
	if sec < tref-secondsPerEra/2 {
		sec += secondsPerEra
	}

	nsec := (int64(t.Fraction)*nanosecondsPerSecond + 1<<31) >> 32

	return time.Unix(sec, nsec).UTC()
}

func (t Time64) Before(u Time64) bool {
	return t.Seconds < u.Seconds ||
		t.Seconds == u.Seconds && t.Fraction < u.Fraction
}

func (t Time64) After(u Time64) bool {
	return t.Seconds > u.Seconds ||
		t.Seconds == u.Seconds && t.Fraction > u.Fraction
}

func ClockOffset(t0, t1, t2, t3 time.Time) time.Duration {
	return (t1.Sub(t0) + t2.Sub(t3)) / 2
}

func RoundTripDelay(t0, t1, t2, t3 time.Time) time.Duration {
	return t3.Sub(t0) - t2.Sub(t1)
}

func EncodePacket(b *[]byte, pkt *Packet) {
	if cap(*b) < PacketLen {
		*b = make([]byte, PacketLen)
	} else {
		*b = (*b)[:PacketLen]
	}

	(*b)[0] = byte(pkt.LVM)
	(*b)[1] = byte(pkt.Stratum)
	(*b)[2] = byte(pkt.Poll)
	(*b)[3] = byte(pkt.Precision)
	binary.BigEndian.PutUint16((*b)[4:], pkt.RootDelay.Seconds)
	binary.BigEndian.PutUint16((*b)[6:], pkt.RootDelay.Fraction)
	binary.BigEndian.PutUint16((*b)[8:], pkt.RootDispersion.Seconds)
	binary.BigEndian.PutUint16((*b)[10:], pkt.RootDispersion.Fraction)
	binary.BigEndian.PutUint32((*b)[12:], pkt.ReferenceID)
	binary.BigEndian.PutUint32((*b)[16:], pkt.ReferenceTime.Seconds)
	binary.BigEndian.PutUint32((*b)[20:], pkt.ReferenceTime.Fraction)
	binary.BigEndian.PutUint32((*b)[24:], pkt.OriginTime.Seconds)
	binary.BigEndian.PutUint32((*b)[28:], pkt.OriginTime.Fraction)
	binary.BigEndian.PutUint32((*b)[32:], pkt.ReceiveTime.Seconds)
	binary.BigEndian.PutUint32((*b)[36:], pkt.ReceiveTime.Fraction)
	binary.BigEndian.PutUint32((*b)[40:], pkt.TransmitTime.Seconds)
	binary.BigEndian.PutUint32((*b)[44:], pkt.TransmitTime.Fraction)
}

func DecodePacket(pkt *Packet, b []byte) error {
	if len(b) < PacketLen {
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

func (p *Packet) LeapIndicator() uint8 {
	return (p.LVM >> 6) & 0b0000_0011
}

func (p *Packet) SetLeapIndicator(l uint8) {
	if l&0b0000_0011 != l {
		panic("unexpected NTP leap indicator value")
	}
	p.LVM = (p.LVM & 0b0011_1111) | (l << 6)
}

func (p *Packet) Version() uint8 {
	return (p.LVM >> 3) & 0b0000_0111
}

func (p *Packet) SetVersion(v uint8) {
	if v&0b0000_0111 != v {
		panic("unexpected NTP version value")
	}
	p.LVM = (p.LVM & 0b_1100_0111) | (v << 3)
}

func (p *Packet) Mode() uint8 {
	return p.LVM & 0b0000_0111
}

func (p *Packet) SetMode(m uint8) {
	if m&0b0000_0111 != m {
		panic("unexpected NTP mode value")
	}
	p.LVM = (p.LVM & 0b1111_1000) | m
}
