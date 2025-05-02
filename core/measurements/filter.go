package measurements

import "time"

type Filter interface {
	Do(cTxTime, sRxTime, sTxTime, cRxTime time.Time) (offset time.Duration)
	Reset()
}

type PreFilter interface {
	Do(cTxTime, sRxTime, sTxTime, cRxTime time.Time) bool
	Reset()
}
