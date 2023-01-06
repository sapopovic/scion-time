package ntp

import (
	"container/heap"
	"errors"
	"sync"
	"time"

	"example.com/scion-time/go/core/timebase"
)

const tssCap = 1 << 20

type tssItem struct {
	key string
	buf [8]struct {
		rxt, txt Time64
	}
	len  int
	qval Time64
	qidx int
}

type tssMap map[string]*tssItem

type tssQueue []*tssItem

var (
	errUnexpectedRequest = errors.New("unexpected request structure")

	tss   = make(tssMap)
	tssQ  = make(tssQueue, 0, tssCap)
	tssMu sync.Mutex
)

func (q tssQueue) Len() int { return len(q) }

func (q tssQueue) Less(i, j int) bool {
	return q[i].qval.Before(q[j].qval)
}

func (q tssQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].qidx = i
	q[j].qidx = j
}

func (q *tssQueue) Push(x any) {
	tssi := x.(*tssItem)
	tssi.qidx = len(*q)
	*q = append(*q, tssi)
}

func (q *tssQueue) Pop() any {
	n := len(*q)
	tssi := (*q)[n-1]
	(*q)[n-1] = nil
	*q = (*q)[0 : n-1]
	return tssi
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

func HandleRequest(clientID string, req *Packet, rxt, txt *time.Time, resp *Packet) {
	resp.SetVersion(VersionMax)
	resp.SetMode(ModeServer)
	resp.Stratum = 1
	resp.Poll = req.Poll
	resp.Precision = -32
	resp.RootDispersion = Time32{Seconds: 0, Fraction: 10}
	resp.ReferenceID = ServerRefID

	*txt = timebase.Now()

	rxt64 := Time64FromTime(*rxt)
	txt64 := Time64FromTime(*txt)

	tssMu.Lock()
	defer tssMu.Unlock()

	var o, min, max int
	tssi, ok := tss[clientID]
	if ok {
		for {
			var i int
			for i, o, min, max = 0, -1, -1, -1; i != tssi.len; i++ {
				if tssi.buf[i].rxt == rxt64 {
					break
				}
				if tssi.buf[i].rxt == req.OriginTime {
					o = i
				}
				if min == -1 || tssi.buf[i].rxt.Before(tssi.buf[min].rxt) {
					min = i
				}
				if max == -1 || !tssi.buf[i].rxt.Before(tssi.buf[max].rxt) {
					max = i
				}
			}
			if i != tssi.len {
				(*rxt).Add(1)
				rxt64 = Time64FromTime(*rxt)
				if !(*rxt).Before(*txt) {
					*txt = *rxt
					(*txt).Add(1)
					txt64 = Time64FromTime(*txt)
				}
				continue
			}
			break
		}
	} else {
		if len(tss) == tssCap && !tssQ[0].qval.After(rxt64) {
			x := heap.Pop(&tssQ).(*tssItem)
			delete(tss, x.key)
		}
		if len(tss) == tssCap {
			tssi = nil
		} else {
			tssi = &tssItem{key: clientID}
			tss[tssi.key] = tssi
			tssi.qval = rxt64
			heap.Push(&tssQ, tssi)
		}
		o, min, max = -1, -1, -1
	}

	resp.ReferenceTime = txt64
	resp.ReceiveTime = rxt64
	if req.ReceiveTime != req.TransmitTime && o != -1 {
		// interleaved mode
		resp.OriginTime = req.ReceiveTime
		resp.TransmitTime = tssi.buf[o].txt
	} else {
		resp.OriginTime = req.TransmitTime
		resp.TransmitTime = txt64
	}

	if tssi != nil {
		if max != -1 && rxt64.After(tssi.buf[max].rxt) {
			tssi.qval = rxt64
			heap.Fix(&tssQ, tssi.qidx)
		}
		if o != -1 {
			tssi.buf[o].rxt = rxt64
			tssi.buf[o].txt = txt64
		} else if tssi.len == cap(tssi.buf) {
			tssi.buf[min].rxt = rxt64
			tssi.buf[min].txt = txt64
		} else {
			tssi.buf[tssi.len].rxt = rxt64
			tssi.buf[tssi.len].txt = txt64
			tssi.len++
		}
	}
}

func UpdateTXTimestamp(clientID string, rxt time.Time, txt *time.Time) {
	if !rxt.Before(*txt) {
		*txt = rxt
		(*txt).Add(1)
	}

	tssMu.Lock()
	defer tssMu.Unlock()

	tssi, ok := tss[clientID]
	if ok {
		rxt64 := Time64FromTime(rxt)
		for i := 0; i != tssi.len; i++ {
			if tssi.buf[i].rxt == rxt64 {
				tssi.buf[i].txt = Time64FromTime(*txt)
				break
			}
		}
	}
}
