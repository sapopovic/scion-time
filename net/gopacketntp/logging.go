package gopacketntp

import (
	"example.com/scion-time/net/ntp"
	"go.uber.org/zap/zapcore"
)

type PacketMarshaler struct {
	Pkt *Packet
}

func (m PacketMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	var err error
	enc.AddUint8("LVM", m.Pkt.LVM)
	enc.AddUint8("Stratum", m.Pkt.Stratum)
	enc.AddInt8("Poll", m.Pkt.Poll)
	enc.AddInt8("Precision", m.Pkt.Precision)
	err = enc.AddObject("RootDelay", ntp.Time32Marshaler{T: m.Pkt.RootDelay})
	if err != nil {
		return err
	}
	err = enc.AddObject("RootDispersion", ntp.Time32Marshaler{T: m.Pkt.RootDispersion})
	if err != nil {
		return err
	}
	enc.AddUint32("ReferenceID", m.Pkt.ReferenceID)
	err = enc.AddObject("ReferenceTime", ntp.Time64Marshaler{T: m.Pkt.ReferenceTime})
	if err != nil {
		return err
	}
	err = enc.AddObject("OriginTime", ntp.Time64Marshaler{T: m.Pkt.OriginTime})
	if err != nil {
		return err
	}
	err = enc.AddObject("ReceiveTime", ntp.Time64Marshaler{T: m.Pkt.ReceiveTime})
	if err != nil {
		return err
	}
	err = enc.AddObject("TransmitTime", ntp.Time64Marshaler{T: m.Pkt.TransmitTime})
	if err != nil {
		return err
	}
	return nil
}
