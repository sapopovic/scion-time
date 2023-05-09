package gopacketntp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/scion-time/net/ntp"
	"github.com/google/gopacket"
)

const (
	nanosecondsPerSecond int64 = 1e9

	ServerPort = 123

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

var LayerTypeNTS = gopacket.RegisterLayerType(
	1213,
	gopacket.LayerTypeMetadata{
		Name:    "NTS",
		Decoder: gopacket.DecodeFunc(decodeNTS),
	},
)

// BaseLayer is a convenience struct which implements the LayerData and
// LayerPayload functions of the Layer interface.
// Copy-pasted from gopacket/layers (we avoid importing this due its massive size)
type BaseLayer struct {
	// Contents is the set of bytes that make up this layer.  IE: for an
	// Ethernet packet, this would be the set of bytes making up the
	// Ethernet frame.
	Contents []byte
	// Payload is the set of bytes contained by (but not part of) this
	// Layer.  Again, to take Ethernet as an example, this would be the
	// set of bytes encapsulated by the Ethernet protocol.
	Payload []byte
}

func (b *BaseLayer) LayerContents() []byte { return b.Contents }

func (b *BaseLayer) LayerPayload() []byte { return b.Payload }

type Packet struct {
	BaseLayer
	LVM            uint8
	Stratum        uint8
	Poll           int8
	Precision      int8
	RootDelay      ntp.Time32
	RootDispersion ntp.Time32
	ReferenceID    uint32
	ReferenceTime  ntp.Time64
	OriginTime     ntp.Time64
	ReceiveTime    ntp.Time64
	TransmitTime   ntp.Time64

	NTSMode            bool
	UniqueID           UniqueIdentifier
	Cookies            []Cookie
	CookiePlaceholders []CookiePlaceholder
	Auth               Authenticator
}

var (
	epoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

	errUnexpectedPacketSize = errors.New("unexpected packet size")
)

func (d *Packet) LayerType() gopacket.LayerType {
	return LayerTypeNTS
}

func decodeNTS(data []byte, p gopacket.PacketBuilder) error {
	d := &Packet{}
	err := d.DecodeFromBytes(data, p)
	if err != nil {
		return err
	}

	p.AddLayer(d)
	p.SetApplicationLayer(d)

	return nil
}

func (pkt *Packet) SerializeTo(b gopacket.SerializeBuffer, opts gopacket.SerializeOptions) error {
	data, err := b.PrependBytes(PacketLen)
	if err != nil {
		return err
	}

	data[0] = byte(pkt.LVM)
	data[1] = byte(pkt.Stratum)
	data[2] = byte(pkt.Poll)
	data[3] = byte(pkt.Precision)
	binary.BigEndian.PutUint16(data[4:], pkt.RootDelay.Seconds)
	binary.BigEndian.PutUint16(data[6:], pkt.RootDelay.Fraction)
	binary.BigEndian.PutUint16(data[8:], pkt.RootDispersion.Seconds)
	binary.BigEndian.PutUint16(data[10:], pkt.RootDispersion.Fraction)
	binary.BigEndian.PutUint32(data[12:], pkt.ReferenceID)
	binary.BigEndian.PutUint32(data[16:], pkt.ReferenceTime.Seconds)
	binary.BigEndian.PutUint32(data[20:], pkt.ReferenceTime.Fraction)
	binary.BigEndian.PutUint32(data[24:], pkt.OriginTime.Seconds)
	binary.BigEndian.PutUint32(data[28:], pkt.OriginTime.Fraction)
	binary.BigEndian.PutUint32(data[32:], pkt.ReceiveTime.Seconds)
	binary.BigEndian.PutUint32(data[36:], pkt.ReceiveTime.Fraction)
	binary.BigEndian.PutUint32(data[40:], pkt.TransmitTime.Seconds)
	binary.BigEndian.PutUint32(data[44:], pkt.TransmitTime.Fraction)

	if pkt.NTSMode {
		buf := new(bytes.Buffer)
		_, _ = buf.Write(data)

		err = pkt.UniqueID.pack(buf)
		if err != nil {
			panic(err)
		}
		for _, c := range pkt.Cookies {
			err = c.pack(buf)
			if err != nil {
				panic(err)
			}
		}
		for _, c := range pkt.CookiePlaceholders {
			err = c.pack(buf)
			if err != nil {
				panic(err)
			}
		}
		err = pkt.Auth.pack(buf)
		if err != nil {
			panic(err)
		}

		ex, err := b.AppendBytes(buf.Len())
		if err != nil {
			return err
		}
		copy(ex, buf.Bytes())
	}

	return nil
}

func (pkt *Packet) DecodeFromBytes(data []byte, df gopacket.DecodeFeedback) error {
	if len(data) < PacketLen {
		df.SetTruncated()
		return errUnexpectedPacketSize
	}

	pkt.BaseLayer = BaseLayer{Contents: data}

	pkt.LVM = uint8(data[0])
	pkt.Stratum = uint8(data[1])
	pkt.Poll = int8(data[2])
	pkt.Precision = int8(data[3])
	pkt.RootDelay.Seconds = binary.BigEndian.Uint16(data[4:])
	pkt.RootDelay.Fraction = binary.BigEndian.Uint16(data[6:])
	pkt.RootDispersion.Seconds = binary.BigEndian.Uint16(data[8:])
	pkt.RootDispersion.Fraction = binary.BigEndian.Uint16(data[10:])
	pkt.ReferenceID = binary.BigEndian.Uint32(data[12:])
	pkt.ReferenceTime.Seconds = binary.BigEndian.Uint32(data[16:])
	pkt.ReferenceTime.Fraction = binary.BigEndian.Uint32(data[20:])
	pkt.OriginTime.Seconds = binary.BigEndian.Uint32(data[24:])
	pkt.OriginTime.Fraction = binary.BigEndian.Uint32(data[28:])
	pkt.ReceiveTime.Seconds = binary.BigEndian.Uint32(data[32:])
	pkt.ReceiveTime.Fraction = binary.BigEndian.Uint32(data[36:])
	pkt.TransmitTime.Seconds = binary.BigEndian.Uint32(data[40:])
	pkt.TransmitTime.Fraction = binary.BigEndian.Uint32(data[44:])

	pos := PacketLen
	msgbuf := bytes.NewReader(data[PacketLen:])
	for msgbuf.Len() >= 28 {
		var eh ExtHdr
		err := eh.unpack(msgbuf)
		if err != nil {
			return fmt.Errorf("unpack extension field: %s", err)
		}

		switch eh.Type {
		case extUniqueIdentifier:
			u := UniqueIdentifier{ExtHdr: eh}
			err = u.unpack(msgbuf)
			if err != nil {
				return fmt.Errorf("unpack UniqueIdentifier: %s", err)
			}
			pkt.UniqueID = u

		case extAuthenticator:
			a := Authenticator{ExtHdr: eh}
			err = a.unpack(msgbuf)
			if err != nil {
				return fmt.Errorf("unpack Authenticator: %s", err)
			}
			a.Pos = pos
			pkt.Auth = a
			break

		case extCookie:
			cookie := Cookie{ExtHdr: eh}
			err = cookie.unpack(msgbuf)
			if err != nil {
				return fmt.Errorf("unpack Cookie: %s", err)
			}
			pkt.Cookies = append(pkt.Cookies, cookie)

		case extCookiePlaceholder:
			cookie := CookiePlaceholder{ExtHdr: eh}
			err = cookie.unpack(msgbuf)
			if err != nil {
				return fmt.Errorf("unpack Cookie: %s", err)
			}
			pkt.CookiePlaceholders = append(pkt.CookiePlaceholders, cookie)

		default:
			// Unknown extension field. Skip it.
			_, err := msgbuf.Seek(int64(eh.Length), io.SeekCurrent)
			if err != nil {
				return err
			}
		}
		pos += int(eh.Length)
	}

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

func (p *Packet) CanDecode() gopacket.LayerClass {
	return LayerTypeNTS
}

func (p *Packet) NextLayerType() gopacket.LayerType {
	return gopacket.LayerTypeZero
}

func (p *Packet) Payload() []byte {
	return nil
}
