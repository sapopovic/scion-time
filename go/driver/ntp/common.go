package ntp

import (
	"errors"
	"log"
	"math"
	"sync"
	"time"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/core/timemath"
)

const (
	ntpLogPrefix = "[driver/ntp]"

	timeout = 1 * time.Second
)

type filterContext struct {
	epoch          uint64
	lo, mid, hi    float64
	alo, amid, ahi float64
	alolo, ahihi   float64
	navg           float64
}

var (
	errUnexpectedPacketFlags     = errors.New("failed to read packet: unexpected flags")
	errUnexpectedPacketStructure = errors.New("failed to read packet: unexpected structure")
	errUnexpectedPacketPayload   = errors.New("failed to read packet: unexpected payload")

	filters = make(map[string]filterContext)
	filtersMu = sync.RWMutex{}
)

func combine(lo, mid, hi time.Duration, trust float64) (offset time.Duration, weight float64) {
	offset = mid
	weight = 0.001 + trust*2.0/timemath.Seconds(hi-lo)
	if weight < 1.0 {
		weight = 1.0
	}
	return
}

func filter(reference string, cTxTime, sRxTime, sTxTime, cRxTime time.Time) (
	offset time.Duration, weight float64) {

	// Based on Ntimed by Poul-Henning Kamp, https://github.com/bsdphk/Ntimed

	filtersMu.RLock()
	f := filters[reference]
	filtersMu.RUnlock()

	lo := timemath.Seconds(cTxTime.Sub(sRxTime))
	hi := timemath.Seconds(cRxTime.Sub(sTxTime))
	mid := (lo + hi) / 2

	if f.epoch != timebase.Epoch() {
		f.epoch = timebase.Epoch()
		f.alo = 0.0
		f.amid = 0.0
		f.ahi = 0.0
		f.alolo = 0.0
		f.ahihi = 0.0
		f.navg = 0.0
	}

	const (
		filterAverage   = 20.0
		filterThreshold = 3.0
	)

	if f.navg < filterAverage {
		f.navg += 1.0
	}

	var loNoise, hiNoise float64
	if f.navg > 2.0 {
		loNoise = math.Sqrt(f.alolo - f.alo*f.alo)
		hiNoise = math.Sqrt(f.ahihi - f.ahi*f.ahi)
	}

	loLim := f.alo - loNoise*filterThreshold
	hiLim := f.ahi + hiNoise*filterThreshold

	var branch int
	failLo := lo < loLim
	failHi := hi > hiLim
	if failLo && failHi {
		branch = 1
	} else if f.navg > 3.0 && failLo {
		mid = f.amid + (hi - f.ahi)
		branch = 2
	} else if f.navg > 3.0 && failHi {
		mid = f.amid + (lo - f.alo)
		branch = 3
	} else {
		branch = 4
	}

	r := f.navg
	if f.navg > 2.0 && branch != 4 {
		r *= r
	}

	f.alo += (lo - f.alo) / r
	f.amid += (mid - f.amid) / r
	f.ahi += (hi - f.ahi) / r
	f.alolo += (lo*lo - f.alolo) / r
	f.ahihi += (hi*hi - f.ahihi) / r

	filtersMu.Lock()
	filters[reference] = f
	filtersMu.Unlock()

	trust := 1.0

	offset, weight = combine(timemath.Duration(lo), timemath.Duration(mid), timemath.Duration(hi), trust)

	log.Printf("%s %s, %v, %fs, %fs, %fs, %fs, %fs, %fs; offeset=%fs, weight=%f",
		ntpLogPrefix, reference, branch,
		lo, mid, hi,
		loLim, f.amid, hiLim,
		timemath.Seconds(offset), weight)

	return timemath.Inv(offset), weight
}
