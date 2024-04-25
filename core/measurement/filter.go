package measurement

import "time"

type Filter interface {
	Do(reference string, cTxTime, sRxTime, sTxTime, cRxTime time.Time) (offset time.Duration)
}
