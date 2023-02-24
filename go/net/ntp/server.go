package ntp

import (
	"container/heap"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

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

	tss        = make(tssMap)
	tssQ       = make(tssQueue, 0, tssCap)
	tssMetrics = struct {
		reqsServedInterleaved prometheus.Counter
		tsRxIncremented       prometheus.Counter
		tsTxIncrementedBefore prometheus.Counter
		tsTxIncrementedAfter  prometheus.Counter
		tssItemsStored        prometheus.Gauge
		tssValuesStored       prometheus.Gauge
	}{
		reqsServedInterleaved: promauto.NewCounter(prometheus.CounterOpts{
			Name: "timeservice_reqs_served_interleaved_total",
			Help: "The total number of requests served in interleaved mode",
		}),
		tsRxIncremented: promauto.NewCounter(prometheus.CounterOpts{
			Name: "timeservice_ts_rx_increments_total",
			Help: "The total number of RX timestamps incremented to ensure monotonicity",
		}),
		tsTxIncrementedBefore: promauto.NewCounter(prometheus.CounterOpts{
			Name: "timeservice_ts_tx_before_increments_total",
			Help: "The total number of TX timestamps incremented before transfer to ensure monotonicity",
		}),
		tsTxIncrementedAfter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "timeservice_ts_tx_after_increments_total",
			Help: "The total number of TX timestamps incremented after transfer to ensure monotonicity",
		}),
		tssItemsStored: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "timeservice_tss_items_stored",
			Help: "The total number of timestamp store items stored (one item per client)",
		}),
		tssValuesStored: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "timeservice_tss_values_stored",
			Help: "The total number of timestamp store values stored",
		}),
	}
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
				// ensure uniqueness of rx timestamps per clientID
				*rxt = rxt.Add(1)
				rxt64 = Time64FromTime(*rxt)
				tssMetrics.tsRxIncremented.Inc()
				if !rxt.Before(*txt) {
					// ensure strict monotonicity of rx/tx timestamps
					*txt = *rxt
					*txt = txt.Add(1)
					txt64 = Time64FromTime(*txt)
					tssMetrics.tsTxIncrementedBefore.Inc()
				}
				continue
			}
			break
		}
	} else {
		if len(tss) == tssCap && !tssQ[0].qval.After(rxt64) {
			// remove minimum timestamp queue item
			x := heap.Pop(&tssQ).(*tssItem)
			delete(tss, x.key)
			tssMetrics.tssItemsStored.Dec()
			tssMetrics.tssValuesStored.Sub(float64(x.len))
		}
		if len(tss) == tssCap {
			tssi = nil
		} else {
			// add timestamp store item
			tssi = &tssItem{key: clientID}
			tss[tssi.key] = tssi
			tssMetrics.tssItemsStored.Inc()
			tssi.qval = rxt64
			heap.Push(&tssQ, tssi)
		}
		o, min, max = -1, -1, -1
	}

	resp.ReferenceTime = txt64
	resp.ReceiveTime = rxt64
	if req.ReceiveTime != req.TransmitTime && o != -1 {
		// interleaved mode: serve from timestamp store
		resp.OriginTime = req.ReceiveTime
		resp.TransmitTime = tssi.buf[o].txt
		tssMetrics.reqsServedInterleaved.Inc()
	} else {
		resp.OriginTime = req.TransmitTime
		resp.TransmitTime = txt64
	}

	if tssi != nil {
		if max != -1 && rxt64.After(tssi.buf[max].rxt) {
			// new maximum rx timestamp, fix queue accordingly
			tssi.qval = rxt64
			heap.Fix(&tssQ, tssi.qidx)
		}
		if o != -1 {
			// maintain interleaved mode timestamp values
			tssi.buf[o].rxt = rxt64
			tssi.buf[o].txt = txt64
		} else if tssi.len == cap(tssi.buf) {
			// replace minimum timestamp values
			tssi.buf[min].rxt = rxt64
			tssi.buf[min].txt = txt64
		} else {
			// add timestamp values
			tssi.buf[tssi.len].rxt = rxt64
			tssi.buf[tssi.len].txt = txt64
			tssi.len++
			tssMetrics.tssValuesStored.Inc()
		}
	}
}

func UpdateTXTimestamp(clientID string, rxt time.Time, txt *time.Time) {
	tssMu.Lock()
	defer tssMu.Unlock()

	if !rxt.Before(*txt) {
		// ensure strict monotonicity of rx/tx timestamps
		*txt = rxt
		*txt = txt.Add(1)
		tssMetrics.tsTxIncrementedAfter.Inc()
	}

	tssi, ok := tss[clientID]
	if ok {
		rxt64 := Time64FromTime(rxt)
		txt64 := Time64FromTime(*txt)
		var i, x, max0, max1 int
		for i, x, max0, max1 = 0, -1, -1, -1; i != tssi.len; i++ {
			if tssi.buf[i].rxt == rxt64 {
				x = i
			}
			if max0 == -1 || !tssi.buf[i].rxt.Before(tssi.buf[max0].rxt) {
				max0, max1 = i, max0
			} else if max1 == -1 || !tssi.buf[i].rxt.Before(tssi.buf[max1].rxt) {
				max1 = i
			}
		}
		if x != -1 {
			if tssi.buf[x].txt != txt64 {
				tssi.buf[x].txt = txt64
			} else {
				// No updated tx timestamp available
				if tssi.len == 1 {
					// remove timestamp store item
					heap.Remove(&tssQ, tssi.qidx)
					delete(tss, tssi.key)
					tssMetrics.tssItemsStored.Dec()
					tssMetrics.tssValuesStored.Sub(float64(tssi.len))
				} else {
					// remove timestamp values
					if tssi.buf[max0].rxt == rxt64 {
						// new maximum rx timestamp, fix queue accordingly
						tssi.qval = tssi.buf[max1].rxt
						heap.Fix(&tssQ, tssi.qidx)
					}
					tssi.buf[x] = tssi.buf[tssi.len-1]
					tssi.len--
					tssMetrics.tssValuesStored.Dec()
				}
			}
		}
	}
}
