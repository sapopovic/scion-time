package scion

import (
	"fmt"

	"go.uber.org/zap/zapcore"

	"github.com/scionproto/scion/pkg/snet"
)

type PathArrayMarshaler struct {
	Paths []snet.Path
}

func (m PathArrayMarshaler) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, p := range m.Paths {
		enc.AppendString(fmt.Sprint(p))
	}
	return nil
}
