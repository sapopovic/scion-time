package ntp

import (
	"errors"
	"sync"
	"time"
)

var (
	errUnexpectedRequest = errors.New("unexpected request structure")

	tss = make(map[Time64]Time64)
	tssMu sync.Mutex
)

func EnsureStrictRxOrder(rxt *time.Time) {
	tssMu.Lock()
	defer tssMu.Unlock()
	_, ok := tss[Time64FromTime(*rxt)]
	for ok {
		(*rxt).Add(1)
		_, ok = tss[Time64FromTime(*rxt)]
	}
	tss[Time64FromTime(*rxt)] = Time64{}
}

func EnsureOrder(t0 time.Time, t1 *time.Time) {
	if (*t1).Sub(t0) < 0 {
		*t1 = t0
	}
}

func StoreTimestamps(rxt, txt time.Time) {
	tssMu.Lock()
	defer tssMu.Unlock()
	t, ok := tss[Time64FromTime(rxt)]
	if !ok || t != (Time64{}) {
		panic("inconsistent timestamps")
	}
	tss[Time64FromTime(rxt)] = Time64FromTime(txt)
}

func ValidateRequest(req *Packet, srcPort uint16) error {
	li := req.LeapIndicator()
	if li != LeapIndicatorNoWarning && li != LeapIndicatorUnknown {
		return errUnexpectedRequest
	}
	vn := req.Version()
	if vn < VersionMin || VersionMax < vn {
		return errUnexpectedRequest
	}
	mode := req.Mode()
	if vn == 1 && mode != ModeReserved0 || vn != 1 && mode != ModeClient {
		return errUnexpectedRequest
	}
	if vn == 1 && srcPort == ServerPort {
		return errUnexpectedRequest
	}
	return nil
}

func HandleRequest(req *Packet, rxt, txt time.Time, resp *Packet) {
	resp.SetVersion(VersionMax)
	resp.SetMode(ModeServer)
	resp.Stratum = 1
	resp.Poll = req.Poll
	resp.Precision = -32
	resp.RootDispersion = Time32{Seconds: 0, Fraction: 10}
	resp.ReferenceID = ServerRefID

	interleaved := false
	var prevtxt Time64
	if req.ReceiveTime != req.TransmitTime {
		tssMu.Lock()
		var ok bool
		prevtxt, ok = tss[req.OriginTime]
		if ok {
			if prevtxt == (Time64{}) {
				panic("inconsistent timestamps")
			}
			delete(tss, req.OriginTime)
			interleaved = true
		}
		tssMu.Unlock()
	}

	resp.ReferenceTime = Time64FromTime(txt)
	resp.ReceiveTime = Time64FromTime(rxt)
	if interleaved {
		resp.OriginTime = req.ReceiveTime
		resp.TransmitTime = prevtxt
	} else {
		resp.OriginTime = req.TransmitTime
		resp.TransmitTime = Time64FromTime(txt)
	}
}
