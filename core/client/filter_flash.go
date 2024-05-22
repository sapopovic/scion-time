package client

// Lucky packet filter combined with median offset filter based on flashptpd,
// https://github.com/meinberg-sync/flashptpd
//
// The filter stores measurements in a FIFO window of configurable capacity and
// picks a predefined number measurements with the lowest round-trip delay
// (lucky packets) assuming that those packets experienced the least amount of
// jitter across the network. Based on the selected set of lucky packets the
// median clock offset value is subsequently calculated and returned as the
// result of each filter step. A filter configuration with a set of exactly one
// lucky packet behaves like a pure lucky packet filter; if the set of lucky
// packets is configured to be equal to the filter's capacity, the resulting
// behavior is equivalent to a pure median offset filter.

import (
	"cmp"
	"slices"
	"time"

	"example.com/scion-time/net/ntp"

	"example.com/scion-time/core/measurements"
)

type measurement struct {
	stamp time.Time
	off   time.Duration
	rtd   time.Duration
}

type LuckyPacketFilter struct {
	pick      int
	state     []measurement
	luckyPkts []measurement
	drift     float64
}

var _ measurements.Filter = (*LuckyPacketFilter)(nil)

func NewLuckyPacketFilter(cap, pick int) *LuckyPacketFilter {
	if cap <= 0 {
		panic("cap must be greater than 0")
	}
	if pick <= 0 {
		panic("pick must be greater than 0")
	}
	return &LuckyPacketFilter{
		pick:      min(pick, cap),
		state:     make([]measurement, 0, cap),
		luckyPkts: make([]measurement, 0, cap),
	}
}

func (f *LuckyPacketFilter) Drift() (float64, bool) {
	return f.drift, len(f.state) >= 2
}

func (f *LuckyPacketFilter) Do(cTxTime, sRxTime, sTxTime, cRxTime time.Time) (
	offset time.Duration) {
	if cap(f.state) == 0 {
		return ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	}
	if len(f.state) == cap(f.state) {
		f.state = f.state[1:]
	}
	f.state = append(f.state, measurement{
		stamp: cTxTime,
		off:   ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime),
		rtd:   ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime),
	})
	var d float64
	if len(f.state) >= 2 {
		for i := 1; i != len(f.state); i++ {
			d += float64((f.state[i].off - f.state[i-1].off).Nanoseconds()) /
				float64(f.state[i].stamp.Sub(f.state[i-1].stamp).Nanoseconds())
		}
		d /= float64(len(f.state) - 1)
	}
	f.drift = d
	f.luckyPkts = f.luckyPkts[:len(f.state)]
	copy(f.luckyPkts, f.state)
	if f.pick < len(f.luckyPkts) {
		slices.SortFunc(f.luckyPkts, func(a, b measurement) int {
			return cmp.Compare(a.rtd, b.rtd)
		})
		f.luckyPkts = f.luckyPkts[:f.pick]
	}
	slices.SortFunc(f.luckyPkts, func(a, b measurement) int {
		return cmp.Compare(a.off, b.off)
	})
	i := len(f.luckyPkts) / 2
	if len(f.luckyPkts)%2 != 0 {
		return f.luckyPkts[i].off
	}
	return f.luckyPkts[i-1].off + (f.luckyPkts[i].off-f.luckyPkts[i-1].off)/2
}

func (f *LuckyPacketFilter) Reset() {
	f.state = f.state[:0]
	f.drift = 0.0
}
