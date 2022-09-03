package core

import (
	"fmt"
	"math"
	"time"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/core/timemath"
)

const stdPLLLogPrefix = "[core/pll_std]"

type StandardPLL struct{
	clk timebase.LocalClock
	epoch uint64
	mode uint64
	t0, t time.Time
	a, b, i float64
}

var _ PLL = (*StandardPLL)(nil)

func NewStandardPLL(clk timebase.LocalClock) *StandardPLL {
	return &StandardPLL{clk: clk}
}

func duration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second) + 0.5)
}

func seconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Second)
}

func (l *StandardPLL) Do(offset time.Duration, weight float64) {
	if l.epoch != l.clk.Epoch() {
		l.epoch = l.clk.Epoch()
		l.mode = 0
	}
	now := l.clk.Now()
	switch l.mode {
	case 0: // startup
		l.t0 = now
		l.mode++
	case 1: // awaiting step
		mdt := now.Sub(l.t0)
		if mdt < 0 {
			panic(fmt.Sprintf("%s unexpected clock behavior", stdPLLLogPrefix))
		}
		if mdt > 2 * time.Second && weight > 3 {
			if timemath.Abs(offset) > 1 * time.Millisecond {
				l.clk.Step(timemath.Inv(offset))
			}
			l.t0 = now
			l.mode++
		}
	case 2: // awaiting PLL
		mdt := now.Sub(l.t0)
		if mdt < 0 {
			panic(fmt.Sprintf("%s unexpected clock behavior", stdPLLLogPrefix))
		}
		if mdt > 6 * time.Second {
			const (
				pInit = 0.33 // initial proportional term
				iInit = 60 // initial p/i ratio
			)
			l.a = pInit
			l.b = l.a / iInit
			l.t0 = now
			l.mode++
		}
	case 3: // tracking
		mdt := now.Sub(l.t0)
		if mdt < 0 {
			panic(fmt.Sprintf("%s unexpected clock behavior", stdPLLLogPrefix))
		}
		ldt := now.Sub(l.t)
		if ldt < 0 {
			panic(fmt.Sprintf("%s unexpected clock behavior", stdPLLLogPrefix))
		}
		var a, b float64
		if weight < 50 {
			a = 3e-2
			b = 5e-4
		} else if weight < 150 {
			a = 6e-2
			b = 1e-3
		} else {
			const (
				captureTime = 300 * time.Second
				stiffenRate = 0.999
				pLimit = 3e-2
			)
			if mdt > captureTime && l.a > pLimit {
				l.a *= math.Pow(stiffenRate, seconds(ldt))
				l.b *= math.Pow(stiffenRate, seconds(ldt))
			}
			a = l.a
			b = l.b
		}
		p := seconds(timemath.Inv(offset)) * a
		d := math.Ceil(seconds(ldt))
		l.i += p * b
		if p > d * 500e-6 {
			p = d * 500e-6
		}
		if p < d * -500e-6 {
			p = d * -500e-6
		}
		if d > 0.0 {
			l.clk.Adjust(duration(p), duration(d), l.i)
		}
	default:
		panic(fmt.Sprintf("%s unexpected mode", stdPLLLogPrefix))
	}
	l.t = now
}
