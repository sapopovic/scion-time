package ntp

import (
	"errors"
	"time"
)

const (
	// Seconds from Unix epoch (1970) to NTP epoch (1900), including 17 leap days
	epoch int64 = -2208988800

	nanosecondsPerSecond int64 = 1e9
	secondsPerEra        int64 = 1 << 32

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
	errUnexpectedPacketSize = errors.New("unexpected packet size")
)

func Time64FromTime(t time.Time) Time64 {
	return Time64{
		Seconds: uint32(
			t.Unix() - epoch),
		// Fraction: uint32(
		// 	(int64(t.Nanosecond())<<32 + nanosecondsPerSecond/2) / nanosecondsPerSecond),
		Fraction: uint32(
			int64(t.Nanosecond()) << 32 / nanosecondsPerSecond),
	}
}

// TimeFromTime64 converts an NTP timestamp to a time.Time using a reference time t0
// to resolve the NTP timestamp era ambiguity.
func TimeFromTime64(t Time64, t0 time.Time) time.Time {
	tref := t0.Unix()

	sec := epoch + (tref-epoch)/secondsPerEra*secondsPerEra + int64(t.Seconds)

	// If the timestamp would be too far in the past relative to
	// the reference time, assume it's from the next era
	if sec < tref-secondsPerEra/2 {
		sec += secondsPerEra
	}

	// nsec := (int64(t.Fraction)*nanosecondsPerSecond + 1<<31) >> 32
	nsec := int64(t.Fraction) * nanosecondsPerSecond >> 32

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

	buf := *b 
	_ = buf[47]
	buf[0] = byte(pkt.LVM)
	buf[1] = byte(pkt.Stratum)
	buf[2] = byte(pkt.Poll)
	buf[3] = byte(pkt.Precision)
	buf[4] = byte(pkt.RootDelay.Seconds >> 8)
	buf[5] = byte(pkt.RootDelay.Seconds)
	buf[6] = byte(pkt.RootDelay.Fraction >> 8)
	buf[7] = byte(pkt.RootDelay.Fraction)
	buf[8] = byte(pkt.RootDispersion.Seconds >> 8)
	buf[9] = byte(pkt.RootDispersion.Seconds)
	buf[10] = byte(pkt.RootDispersion.Fraction >> 8)
	buf[11] = byte(pkt.RootDispersion.Fraction)
	buf[12] = byte(pkt.ReferenceID >> 24)
	buf[13] = byte(pkt.ReferenceID >> 16)
	buf[14] = byte(pkt.ReferenceID >> 8)
	buf[15] = byte(pkt.ReferenceID)
	buf[16] = byte(pkt.ReferenceTime.Seconds >> 24)
	buf[17] = byte(pkt.ReferenceTime.Seconds >> 16)
	buf[18] = byte(pkt.ReferenceTime.Seconds >> 8)
	buf[19] = byte(pkt.ReferenceTime.Seconds)
	buf[20] = byte(pkt.ReferenceTime.Fraction >> 24)
	buf[21] = byte(pkt.ReferenceTime.Fraction >> 16)
	buf[22] = byte(pkt.ReferenceTime.Fraction >> 8)
	buf[23] = byte(pkt.ReferenceTime.Fraction)
	buf[24] = byte(pkt.OriginTime.Seconds >> 24)
	buf[25] = byte(pkt.OriginTime.Seconds >> 16)
	buf[26] = byte(pkt.OriginTime.Seconds >> 8)
	buf[27] = byte(pkt.OriginTime.Seconds)
	buf[28] = byte(pkt.OriginTime.Fraction >> 24)
	buf[29] = byte(pkt.OriginTime.Fraction >> 16)
	buf[30] = byte(pkt.OriginTime.Fraction >> 8)
	buf[31] = byte(pkt.OriginTime.Fraction)
	buf[32] = byte(pkt.ReceiveTime.Seconds >> 24)
	buf[33] = byte(pkt.ReceiveTime.Seconds >> 16)
	buf[34] = byte(pkt.ReceiveTime.Seconds >> 8)
	buf[35] = byte(pkt.ReceiveTime.Seconds)
	buf[36] = byte(pkt.ReceiveTime.Fraction >> 24)
	buf[37] = byte(pkt.ReceiveTime.Fraction >> 16)
	buf[38] = byte(pkt.ReceiveTime.Fraction >> 8)
	buf[39] = byte(pkt.ReceiveTime.Fraction)
	buf[40] = byte(pkt.TransmitTime.Seconds >> 24)
	buf[41] = byte(pkt.TransmitTime.Seconds >> 16)
	buf[42] = byte(pkt.TransmitTime.Seconds >> 8)
	buf[43] = byte(pkt.TransmitTime.Seconds)
	buf[44] = byte(pkt.TransmitTime.Fraction >> 24)
	buf[45] = byte(pkt.TransmitTime.Fraction >> 16)
	buf[46] = byte(pkt.TransmitTime.Fraction >> 8)
	buf[47] = byte(pkt.TransmitTime.Fraction)
}

func DecodePacket(pkt *Packet, b []byte) error {
	if len(b) < PacketLen {
		return errUnexpectedPacketSize
	}

	_ = b[47]
	pkt.LVM = uint8(b[0])
	pkt.Stratum = uint8(b[1])
	pkt.Poll = int8(b[2])
	pkt.Precision = int8(b[3])
	pkt.RootDelay.Seconds = uint16(b[4])<<8 | uint16(b[5])
	pkt.RootDelay.Fraction = uint16(b[6])<<8 | uint16(b[7])
	pkt.RootDispersion.Seconds = uint16(b[8])<<8 | uint16(b[9])
	pkt.RootDispersion.Fraction = uint16(b[10])<<8 | uint16(b[11])
	pkt.ReferenceID = uint32(b[12])<<24 | uint32(b[13])<<16 | uint32(b[14])<<8 | uint32(b[15])
	pkt.ReferenceTime.Seconds = uint32(b[16])<<24 | uint32(b[17])<<16 | uint32(b[18])<<8 | uint32(b[19])
	pkt.ReferenceTime.Fraction = uint32(b[20])<<24 | uint32(b[21])<<16 | uint32(b[22])<<8 | uint32(b[23])
	pkt.OriginTime.Seconds = uint32(b[24])<<24 | uint32(b[25])<<16 | uint32(b[26])<<8 | uint32(b[27])
	pkt.OriginTime.Fraction = uint32(b[28])<<24 | uint32(b[29])<<16 | uint32(b[30])<<8 | uint32(b[31])
	pkt.ReceiveTime.Seconds = uint32(b[32])<<24 | uint32(b[33])<<16 | uint32(b[34])<<8 | uint32(b[35])
	pkt.ReceiveTime.Fraction = uint32(b[36])<<24 | uint32(b[37])<<16 | uint32(b[38])<<8 | uint32(b[39])
	pkt.TransmitTime.Seconds = uint32(b[40])<<24 | uint32(b[41])<<16 | uint32(b[42])<<8 | uint32(b[43])
	pkt.TransmitTime.Fraction = uint32(b[44])<<24 | uint32(b[45])<<16 | uint32(b[46])<<8 | uint32(b[47])

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
