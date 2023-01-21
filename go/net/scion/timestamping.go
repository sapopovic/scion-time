package scion

import (
	"github.com/scionproto/scion/pkg/slayers"
)

const OptTypeTimestamp = 253 // experimental

type TimestampOption struct {
	*slayers.EndToEndOption
}
