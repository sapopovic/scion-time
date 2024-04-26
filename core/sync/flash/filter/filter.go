package filter

import (
	"time"

	"example.com/scion-time/net/ntp"

	"example.com/scion-time/core/measurement"
)

const (
	defaultFilterSize = 16
)

type filterItem struct {
	off time.Duration
	rtd time.Duration
}

type filter struct {
	reference string
	state     []filterItem
}

func NewLuckyPacketFilter() measurement.Filter {
	return &filter{}
}

func (f *filter) Do(reference string, cTxTime, sRxTime, sTxTime, cRxTime time.Time) (
	offset time.Duration) {
	if reference == "" {
		panic("invalid argument: reference must not be \"\"")
	}
	if reference != f.reference {
		if f.reference != "" {
			panic("filter must be used with a single reference")
		}
		f.reference = reference
	}
	if len(f.state) == defaultFilterSize {
		f.state = f.state[1:]
	}
	f.state = append(f.state, filterItem{
		off: ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime),
		rtd: ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime),
	})
	return f.state[len(f.state)-1].off
}
