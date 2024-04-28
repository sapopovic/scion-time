package filter

import (
	"sort"
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
	cap, pick int
	reference string
	state     []filterItem
	luckyPkts []filterItem
}

func NewLuckyPacketFilter(cap, pick int) measurement.Filter {
	if pick > cap {
		panic("pick must not be greater than cap")
	}
	return &filter{cap: cap, pick: pick}
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
		f.state = make([]filterItem, 0, f.cap)
		f.luckyPkts = make([]filterItem, 0, f.cap)
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
	sort.Slice(f.luckyPkts, func(i, j int) bool {
		return f.luckyPkts[i].rtd < f.luckyPkts[j].rtd
	})
	f.luckyPkts = f.luckyPkts[:min(f.pick, len(f.luckyPkts))]
	sort.Slice(f.luckyPkts, func(i, j int) bool {
		return f.luckyPkts[i].off < f.luckyPkts[j].off
	})
	i := len(f.luckyPkts) / 2
	if len(f.luckyPkts)%2 != 0 {
		return f.luckyPkts[i].off
	}
	return f.luckyPkts[i-1].off + (f.luckyPkts[i].off-f.luckyPkts[i-1].off)/2
}
