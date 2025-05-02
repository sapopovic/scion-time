package client

import (
	"context"
	"log/slog"
	"math"
	"time"

	"example.com/scion-time/core/measurements"
	"example.com/scion-time/net/ntp"
)

type AvgPreFilter struct {
	log     *slog.Logger
	logCtx  context.Context
	offsets [60]time.Duration // fixed-size ring buffer
	sum     time.Duration     // running sum
	index   int               // current index in buffer
	count   int
	avg     time.Duration
}

var _ measurements.PreFilter = (*AvgPreFilter)(nil)

func NewAvgPreFilter(log *slog.Logger) *AvgPreFilter {
	log.Info("Average PreFilter activated")
	return &AvgPreFilter{log: log, logCtx: context.Background()}
}

func (f *AvgPreFilter) updateOffsetAverage(newOffset time.Duration) time.Duration {
	if f.count < len(f.offsets) {
		f.sum += newOffset
		f.offsets[f.index] = newOffset
		f.count++
	} else {
		f.sum -= f.offsets[f.index]
		f.sum += newOffset
		f.offsets[f.index] = newOffset
	}
	f.index = (f.index + 1) % len(f.offsets)

	if f.count == 0 {
		return 0
	}
	return f.sum / time.Duration(f.count)
}

func (f *AvgPreFilter) isOutlier(newOffset time.Duration) bool {

	sumSquares := time.Duration(0)
	for i := 0; i < f.count; i++ {
		diff := f.offsets[i] - f.avg
		sumSquares += diff * diff
	}
	stdDev := time.Duration(math.Sqrt(float64(sumSquares) / float64(f.count)))

	if time.Duration(math.Abs(float64(newOffset-f.avg))) <= 2*stdDev {
		return false
	}

	return true
}

func (f *AvgPreFilter) Do(cTxTime, sRxTime, sTxTime, cRxTime time.Time) bool {

	rawOffset := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	if f.isOutlier(rawOffset) { // if its an outlier --> IGNORE, but update average
		f.updateOffsetAverage(rawOffset)
		return true
	}
	f.updateOffsetAverage(rawOffset)
	return false
}

func (f *AvgPreFilter) Reset() {
}
