package measurements

import "time"

type Filter interface {
	Do(cTxTime, sRxTime, sTxTime, cRxTime time.Time) (lo, mid, hi time.Duration)
	Reset()
}
