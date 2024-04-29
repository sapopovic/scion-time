package filter

import (
	"cmp"
	"slices"
	"time"

	"example.com/scion-time/net/ntp"

	"example.com/scion-time/core/measurement"
)

const (
	DefaultCapacity = 16
	DefaultPick     = 7
)

type filterItem struct {
	off time.Duration
	rtd time.Duration
}

type filter struct {
	pick      int
	state     []filterItem
	luckyPkts []filterItem
	reference string
}

func NewLuckyPacketFilter(cap, pick int) measurement.Filter {
	if cap == 0 {
		panic("cap must not be zero")
	}
	if pick > cap {
		panic("pick must not be greater than cap")
	}
	return &filter{
		pick:      pick,
		state:     make([]filterItem, 0, cap),
		luckyPkts: make([]filterItem, 0, cap),
	}
}

func (f *filter) Do(reference string, cTxTime, sRxTime, sTxTime, cRxTime time.Time) (
	offset time.Duration) {
	if reference == "" {
		panic("reference must not be empty")
	}
	if reference != f.reference {
		if f.reference != "" {
			panic("filter must be used with a single reference")
		}
		f.reference = reference
	}
	if len(f.state) == cap(f.state) {
		f.state = f.state[1:]
	}
	f.state = append(f.state, filterItem{
		off: ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime),
		rtd: ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime),
	})
	f.luckyPkts = f.luckyPkts[:len(f.state)]
	copy(f.luckyPkts, f.state)
	if f.pick < len(f.luckyPkts) {
		slices.SortFunc(f.luckyPkts, func(a, b filterItem) int {
			return cmp.Compare(a.rtd, b.rtd)
		})
		f.luckyPkts = f.luckyPkts[:f.pick]
	}
	slices.SortFunc(f.luckyPkts, func(a, b filterItem) int {
		return cmp.Compare(a.off, b.off)
	})
	i := len(f.luckyPkts) / 2
	if len(f.luckyPkts)%2 != 0 {
		return f.luckyPkts[i].off
	}
	return f.luckyPkts[i-1].off + (f.luckyPkts[i].off-f.luckyPkts[i-1].off)/2
}
