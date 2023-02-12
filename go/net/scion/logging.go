package scion

import (
	"go.uber.org/zap/zapcore"

	"github.com/scionproto/scion/pkg/snet"
)

type PathInterfaceMarshaler struct {
	PathInterface snet.PathInterface
}

func (m PathInterfaceMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("IA", m.PathInterface.IA.String())
	enc.AddUint64("ID", uint64(m.PathInterface.ID))
	return nil
}

type PathInterfaceArrayMarshaler struct {
	PathInterfaces []snet.PathInterface
}

func (m PathInterfaceArrayMarshaler) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, i := range m.PathInterfaces {
		i := i
		enc.AppendObject(PathInterfaceMarshaler{PathInterface: i})
	}
	return nil
}

type PathMarshaler struct {
	Path snet.Path
}

func (m PathMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	p := m.Path
	md := p.Metadata()
	enc.AddArray("hops", PathInterfaceArrayMarshaler{PathInterfaces: md.Interfaces})
	enc.AddUint16("MTU", md.MTU)
	var nh string
	unh := p.UnderlayNextHop()
	if unh != nil {
		nh = unh.String()
	} else {
		nh = ""
	}
	enc.AddString("NextHop", nh)
	return nil
}

type PathArrayMarshaler struct {
	Paths []snet.Path
}

func (m PathArrayMarshaler) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, p := range m.Paths {
		p := p
		enc.AppendObject(PathMarshaler{Path: p})
	}
	return nil
}
