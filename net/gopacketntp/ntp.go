package gopacketntp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"example.com/scion-time/net/ntp"
	"github.com/google/gopacket"
)

const (
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
	errUnexpectedPacketSize = errors.New("unexpected packet size")
)

func (p *Packet) LayerType() gopacket.LayerType {
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

func (p *Packet) SerializeTo(b gopacket.SerializeBuffer, opts gopacket.SerializeOptions) error {
	data, err := b.PrependBytes(PacketLen)
	if err != nil {
		return err
	}

	data[0] = byte(p.LVM)
	data[1] = byte(p.Stratum)
	data[2] = byte(p.Poll)
	data[3] = byte(p.Precision)
	binary.BigEndian.PutUint16(data[4:], p.RootDelay.Seconds)
	binary.BigEndian.PutUint16(data[6:], p.RootDelay.Fraction)
	binary.BigEndian.PutUint16(data[8:], p.RootDispersion.Seconds)
	binary.BigEndian.PutUint16(data[10:], p.RootDispersion.Fraction)
	binary.BigEndian.PutUint32(data[12:], p.ReferenceID)
	binary.BigEndian.PutUint32(data[16:], p.ReferenceTime.Seconds)
	binary.BigEndian.PutUint32(data[20:], p.ReferenceTime.Fraction)
	binary.BigEndian.PutUint32(data[24:], p.OriginTime.Seconds)
	binary.BigEndian.PutUint32(data[28:], p.OriginTime.Fraction)
	binary.BigEndian.PutUint32(data[32:], p.ReceiveTime.Seconds)
	binary.BigEndian.PutUint32(data[36:], p.ReceiveTime.Fraction)
	binary.BigEndian.PutUint32(data[40:], p.TransmitTime.Seconds)
	binary.BigEndian.PutUint32(data[44:], p.TransmitTime.Fraction)

	if p.NTSMode {
		buf := new(bytes.Buffer)
		_, _ = buf.Write(data)

		err = p.UniqueID.pack(buf)
		if err != nil {
			panic(err)
		}
		for _, c := range p.Cookies {
			err = c.pack(buf)
			if err != nil {
				panic(err)
			}
		}
		for _, c := range p.CookiePlaceholders {
			err = c.pack(buf)
			if err != nil {
				panic(err)
			}
		}
		err = p.Auth.pack(buf)
		if err != nil {
			panic(err)
		}

		ex, err := b.AppendBytes(buf.Len() - PacketLen)
		if err != nil {
			return err
		}
		copy(ex, buf.Bytes()[PacketLen:])
	}

	return nil
}

func (p *Packet) DecodeFromBytes(data []byte, df gopacket.DecodeFeedback) error {
	if len(data) < PacketLen {
		df.SetTruncated()
		return errUnexpectedPacketSize
	}

	p.BaseLayer = BaseLayer{Contents: data}

	p.LVM = uint8(data[0])
	p.Stratum = uint8(data[1])
	p.Poll = int8(data[2])
	p.Precision = int8(data[3])
	p.RootDelay.Seconds = binary.BigEndian.Uint16(data[4:])
	p.RootDelay.Fraction = binary.BigEndian.Uint16(data[6:])
	p.RootDispersion.Seconds = binary.BigEndian.Uint16(data[8:])
	p.RootDispersion.Fraction = binary.BigEndian.Uint16(data[10:])
	p.ReferenceID = binary.BigEndian.Uint32(data[12:])
	p.ReferenceTime.Seconds = binary.BigEndian.Uint32(data[16:])
	p.ReferenceTime.Fraction = binary.BigEndian.Uint32(data[20:])
	p.OriginTime.Seconds = binary.BigEndian.Uint32(data[24:])
	p.OriginTime.Fraction = binary.BigEndian.Uint32(data[28:])
	p.ReceiveTime.Seconds = binary.BigEndian.Uint32(data[32:])
	p.ReceiveTime.Fraction = binary.BigEndian.Uint32(data[36:])
	p.TransmitTime.Seconds = binary.BigEndian.Uint32(data[40:])
	p.TransmitTime.Fraction = binary.BigEndian.Uint32(data[44:])

	pos := PacketLen
	msgbuf := bytes.NewReader(data[PacketLen:])
	foundAuthenticator := false
	for msgbuf.Len() >= 28 && !foundAuthenticator {
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
			p.UniqueID = u

		case extAuthenticator:
			a := Authenticator{ExtHdr: eh}
			err = a.unpack(msgbuf)
			if err != nil {
				return fmt.Errorf("unpack Authenticator: %s", err)
			}
			a.Pos = pos
			p.Auth = a
			foundAuthenticator = true

		case extCookie:
			cookie := Cookie{ExtHdr: eh}
			err = cookie.unpack(msgbuf)
			if err != nil {
				return fmt.Errorf("unpack Cookie: %s", err)
			}
			p.Cookies = append(p.Cookies, cookie)

		case extCookiePlaceholder:
			cookie := CookiePlaceholder{ExtHdr: eh}
			err = cookie.unpack(msgbuf)
			if err != nil {
				return fmt.Errorf("unpack Cookie: %s", err)
			}
			p.CookiePlaceholders = append(p.CookiePlaceholders, cookie)

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
